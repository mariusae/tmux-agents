package app

import (
	"context"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/mariusae/tmux-agents/internal/model"
	"github.com/mariusae/tmux-agents/internal/reconcile"
	"github.com/mariusae/tmux-agents/internal/store"
	"github.com/mariusae/tmux-agents/internal/tmux"
)

type App struct {
	store store.Store
}

type RecordRequest struct {
	Provider          string
	ProviderSessionID string
	Kind              string
	Message           string
	Source            string
}

func OpenDefault() (*App, error) {
	dbPath, err := defaultDBPath()
	if err != nil {
		return nil, err
	}
	return Open(dbPath)
}

func Open(dbPath string) (*App, error) {
	st, err := store.OpenBolt(dbPath)
	if err != nil {
		return nil, err
	}
	return &App{store: st}, nil
}

func (a *App) Close() error {
	if a == nil || a.store == nil {
		return nil
	}
	return a.store.Close()
}

func (a *App) Record(ctx context.Context, req RecordRequest) (model.Event, model.Agent, error) {
	kind, err := model.ParseEventKind(req.Kind)
	if err != nil {
		return model.Event{}, model.Agent{}, err
	}
	source, err := model.ParseEventSource(req.Source)
	if err != nil {
		return model.Event{}, model.Agent{}, err
	}

	tmuxInfo, err := tmux.DiscoverCurrentContext(ctx)
	if err != nil {
		return model.Event{}, model.Agent{}, err
	}

	event := model.Event{
		Time:              time.Now().UTC(),
		Provider:          strings.TrimSpace(req.Provider),
		ProviderSessionID: normalizeProviderSessionID(strings.TrimSpace(req.ProviderSessionID), tmuxInfo.PaneID),
		TmuxSession:       tmuxInfo.Session,
		TmuxWindow:        tmuxInfo.Window,
		TmuxPane:          tmuxInfo.Pane,
		Kind:              kind,
		Message:           strings.TrimSpace(req.Message),
		Source:            source,
	}

	return a.store.RecordEvent(ctx, event)
}

func (a *App) ListEvents(ctx context.Context, afterSeq uint64, limit int) ([]model.Event, error) {
	return a.store.ListEvents(ctx, afterSeq, limit)
}

func (a *App) WaitingAgents(ctx context.Context) ([]model.Agent, error) {
	_ = a.reconcileIfStale(ctx, 1*time.Second)

	agents, err := a.store.ListAgents(ctx)
	if err != nil {
		return nil, err
	}

	waiting := make([]model.Agent, 0, len(agents))
	for _, agent := range agents {
		if agent.AwaitingInput && agent.State != model.AgentStateGone {
			waiting = append(waiting, agent)
		}
	}

	return sortAgents(dedupeAgents(waiting)), nil
}

func (a *App) Agents(ctx context.Context) ([]model.Agent, error) {
	_ = a.reconcileIfStale(ctx, 1*time.Second)

	agents, err := a.store.ListAgents(ctx)
	if err != nil {
		return nil, err
	}
	return sortAgents(dedupeAgents(agents)), nil
}

func (a *App) StatusLine(ctx context.Context) (string, error) {
	waiting, err := a.WaitingAgents(ctx)
	if err != nil {
		return "", err
	}
	if len(waiting) == 0 {
		return "", nil
	}

	labels := make([]string, 0, len(waiting))
	for _, agent := range waiting {
		labels = append(labels, agent.Label())
	}

	return strings.Join(labels, " "), nil
}

func (a *App) Reconcile(ctx context.Context) (reconcile.Result, error) {
	return reconcile.Run(ctx, a.store)
}

func defaultDBPath() (string, error) {
	if path := strings.TrimSpace(os.Getenv("TMUX_AGENTS_DB_PATH")); path != "" {
		return path, nil
	}

	cacheDir, err := os.UserCacheDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(cacheDir, "tmux-agents", "tmux-agents.db"), nil
}

func lastSortTime(agent model.Agent) time.Time {
	return agent.LastActivityAt()
}

func (a *App) reconcileIfStale(ctx context.Context, maxAge time.Duration) error {
	last, err := a.store.GetMeta(ctx, "last_reconcile_at")
	if err != nil {
		return err
	}
	if last != "" {
		timestamp, err := time.Parse(time.RFC3339Nano, last)
		if err == nil && time.Since(timestamp) < maxAge {
			return nil
		}
	}

	_, err = a.Reconcile(ctx)
	return err
}

func sortAgents(agents []model.Agent) []model.Agent {
	sort.Slice(agents, func(i, j int) bool {
		return lastSortTime(agents[i]).After(lastSortTime(agents[j]))
	})
	return agents
}

func normalizeProviderSessionID(providerSessionID, tmuxPaneID string) string {
	if providerSessionID != "" {
		return providerSessionID
	}
	if strings.TrimSpace(tmuxPaneID) != "" {
		return "pane:" + strings.TrimSpace(tmuxPaneID)
	}
	return ""
}

func dedupeAgents(agents []model.Agent) []model.Agent {
	byLabel := make(map[string]model.Agent, len(agents))
	order := make([]string, 0, len(agents))

	for _, agent := range agents {
		key := agent.Label()
		if key == "unknown" {
			key = agent.Key
		}

		existing, exists := byLabel[key]
		if !exists {
			byLabel[key] = agent
			order = append(order, key)
			continue
		}

		if prefersAgent(agent, existing) {
			byLabel[key] = agent
		}
	}

	out := make([]model.Agent, 0, len(byLabel))
	for _, key := range order {
		out = append(out, byLabel[key])
	}
	return out
}

func prefersAgent(candidate, existing model.Agent) bool {
	if candidate.Live != existing.Live {
		return candidate.Live
	}
	if candidate.AwaitingInput != existing.AwaitingInput {
		return candidate.AwaitingInput
	}
	return lastSortTime(candidate).After(lastSortTime(existing))
}
