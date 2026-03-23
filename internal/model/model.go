package model

import (
	"fmt"
	"strings"
	"time"
)

type EventKind string

const (
	EventKindPromptSubmitted    EventKind = "prompt_submitted"
	EventKindToolStarted        EventKind = "tool_started"
	EventKindToolFinished       EventKind = "tool_finished"
	EventKindTurnCompleted      EventKind = "turn_completed"
	EventKindNotification       EventKind = "notification"
	EventKindLiveDetected       EventKind = "live_detected"
	EventKindLiveMissing        EventKind = "live_missing"
	EventKindPaneChanged        EventKind = "pane_changed"
	EventKindPaneClosed         EventKind = "pane_closed"
	EventKindPaneMoved          EventKind = "pane_moved"
	EventKindManualNote         EventKind = "manual_note"
	EventKindStateRunning       EventKind = "state_running"
	EventKindStateAwaitingInput EventKind = "state_awaiting_input"
	EventKindStateIdle          EventKind = "state_idle"
	EventKindStateGone          EventKind = "state_gone"
)

type EventSource string

const (
	EventSourceHook      EventSource = "hook"
	EventSourceReconcile EventSource = "reconcile"
	EventSourceUser      EventSource = "user"
	EventSourceSystem    EventSource = "system"
)

type AgentState string

const (
	AgentStateRunning       AgentState = "running"
	AgentStateAwaitingInput AgentState = "awaiting_input"
	AgentStateIdle          AgentState = "idle"
	AgentStateGone          AgentState = "gone"
	AgentStateUnknown       AgentState = "unknown"
)

type Event struct {
	Seq               uint64            `json:"seq"`
	Time              time.Time         `json:"time"`
	Provider          string            `json:"provider"`
	ProviderSessionID string            `json:"provider_session_id"`
	TmuxSession       string            `json:"tmux_session,omitempty"`
	TmuxWindow        string            `json:"tmux_window,omitempty"`
	TmuxWindowName    string            `json:"tmux_window_name,omitempty"`
	TmuxPane          string            `json:"tmux_pane,omitempty"`
	Kind              EventKind         `json:"kind"`
	Message           string            `json:"message,omitempty"`
	Source            EventSource       `json:"source"`
	Metadata          map[string]string `json:"metadata,omitempty"`
}

func (e Event) AgentKey() string {
	if e.Provider == "" || e.ProviderSessionID == "" {
		return ""
	}
	return AgentKey(e.Provider, e.ProviderSessionID)
}

type Agent struct {
	Key               string     `json:"key"`
	Provider          string     `json:"provider"`
	ProviderSessionID string     `json:"provider_session_id"`
	TmuxSession       string     `json:"tmux_session,omitempty"`
	TmuxWindow        string     `json:"tmux_window,omitempty"`
	TmuxWindowName    string     `json:"tmux_window_name,omitempty"`
	TmuxPane          string     `json:"tmux_pane,omitempty"`
	State             AgentState `json:"state"`
	AwaitingInput     bool       `json:"awaiting_input"`
	Live              bool       `json:"live"`
	LastEventAt       time.Time  `json:"last_event_at"`
	LastActiveAt      time.Time  `json:"last_active_at"`
	LastSeenAt        time.Time  `json:"last_seen_at"`
	StateChangedAt    time.Time   `json:"state_changed_at"`
	StateSource       EventSource `json:"state_source,omitempty"`
	ReconcileSource   string      `json:"reconcile_source,omitempty"`
}

func (a Agent) Label() string {
	if a.Provider == "" {
		return "unknown"
	}
	if target := a.TargetLabel(); target != "" {
		return fmt.Sprintf("%s@%s", a.Provider, target)
	}
	if a.ProviderSessionID == "" {
		return a.Provider
	}
	return fmt.Sprintf("%s/%s", a.Provider, a.ProviderSessionID)
}

func (a Agent) TargetLabel() string {
	session := strings.TrimSpace(a.TmuxSession)
	window := normalizeWindowIndex(a.TmuxWindow)
	pane := normalizePaneIndex(a.TmuxPane)
	if session == "" || window == "" || pane == "" {
		return ""
	}
	windowDisplay := window
	if name := strings.TrimSpace(a.TmuxWindowName); name != "" {
		windowDisplay = name
	}
	return fmt.Sprintf("%s:%s.%s", session, windowDisplay, pane)
}

func (a Agent) LocationLabel() string {
	if target := a.TargetLabel(); target != "" {
		return target
	}
	return "-"
}

func (a Agent) LastActivityAt() time.Time {
	candidates := []time.Time{a.LastActiveAt, a.LastEventAt, a.LastSeenAt}
	best := time.Time{}
	for _, candidate := range candidates {
		if candidate.After(best) {
			best = candidate
		}
	}
	return best
}

func AgentKey(provider, providerSessionID string) string {
	return fmt.Sprintf("%s:%s", strings.TrimSpace(provider), strings.TrimSpace(providerSessionID))
}

func normalizeWindowIndex(raw string) string {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return ""
	}
	if head, _, found := strings.Cut(trimmed, ":"); found {
		return head
	}
	return trimmed
}

func normalizePaneIndex(raw string) string {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" || strings.HasPrefix(trimmed, "%") {
		return ""
	}
	return trimmed
}

func ParseEventKind(raw string) (EventKind, error) {
	normalized := strings.ToLower(strings.TrimSpace(raw))
	switch normalized {
	case string(EventKindPromptSubmitted), "prompt":
		return EventKindPromptSubmitted, nil
	case string(EventKindToolStarted):
		return EventKindToolStarted, nil
	case string(EventKindToolFinished):
		return EventKindToolFinished, nil
	case string(EventKindTurnCompleted), "done":
		return EventKindTurnCompleted, nil
	case string(EventKindNotification):
		return EventKindNotification, nil
	case string(EventKindLiveDetected), "running":
		return EventKindLiveDetected, nil
	case string(EventKindLiveMissing), "missing":
		return EventKindLiveMissing, nil
	case string(EventKindPaneChanged):
		return EventKindPaneChanged, nil
	case string(EventKindPaneClosed):
		return EventKindPaneClosed, nil
	case string(EventKindPaneMoved):
		return EventKindPaneMoved, nil
	case string(EventKindManualNote), "note":
		return EventKindManualNote, nil
	case string(EventKindStateRunning):
		return EventKindStateRunning, nil
	case string(EventKindStateAwaitingInput), "awaiting_input", "waiting":
		return EventKindStateAwaitingInput, nil
	case string(EventKindStateIdle), "idle":
		return EventKindStateIdle, nil
	case string(EventKindStateGone), "gone":
		return EventKindStateGone, nil
	default:
		return "", fmt.Errorf("unknown event kind %q", raw)
	}
}

func ParseEventSource(raw string) (EventSource, error) {
	normalized := strings.ToLower(strings.TrimSpace(raw))
	switch normalized {
	case "", string(EventSourceUser):
		return EventSourceUser, nil
	case string(EventSourceHook):
		return EventSourceHook, nil
	case string(EventSourceReconcile):
		return EventSourceReconcile, nil
	case string(EventSourceSystem):
		return EventSourceSystem, nil
	default:
		return "", fmt.Errorf("unknown event source %q", raw)
	}
}

func ApplyEvent(agent Agent, event Event) Agent {
	if agent.Key == "" {
		agent.Key = event.AgentKey()
	}
	if agent.Provider == "" {
		agent.Provider = event.Provider
	}
	if agent.ProviderSessionID == "" {
		agent.ProviderSessionID = event.ProviderSessionID
	}
	if event.TmuxSession != "" {
		agent.TmuxSession = event.TmuxSession
	}
	if event.TmuxWindow != "" {
		agent.TmuxWindow = event.TmuxWindow
	}
	if event.TmuxWindowName != "" {
		agent.TmuxWindowName = event.TmuxWindowName
	}
	if event.TmuxPane != "" {
		agent.TmuxPane = event.TmuxPane
	}
	if !event.Time.IsZero() {
		agent.LastEventAt = event.Time
		agent.LastSeenAt = event.Time
	}

	prevState := agent.State

	// Reconcile events confirm liveness but defer to hook-derived state.
	reconcileDefers := event.Source == EventSourceReconcile &&
		agent.StateSource == EventSourceHook &&
		event.Kind != EventKindLiveMissing &&
		event.Kind != EventKindPaneClosed &&
		event.Kind != EventKindStateGone

	if reconcileDefers {
		agent.Live = true
		agent.LastSeenAt = event.Time
	} else {
		switch event.Kind {
		case EventKindPromptSubmitted, EventKindToolStarted, EventKindLiveDetected, EventKindStateRunning:
			agent.State = AgentStateRunning
			agent.AwaitingInput = false
			agent.Live = true
			agent.LastActiveAt = event.Time
		case EventKindToolFinished:
			agent.State = AgentStateRunning
			agent.AwaitingInput = false
			agent.Live = true
			agent.LastActiveAt = event.Time
		case EventKindTurnCompleted, EventKindNotification, EventKindStateAwaitingInput:
			agent.State = AgentStateAwaitingInput
			agent.AwaitingInput = true
			agent.Live = true
		case EventKindStateIdle:
			agent.State = AgentStateIdle
			agent.AwaitingInput = false
			agent.Live = true
		case EventKindLiveMissing, EventKindPaneClosed, EventKindStateGone:
			agent.State = AgentStateGone
			agent.AwaitingInput = false
			agent.Live = false
		case EventKindPaneChanged, EventKindPaneMoved:
			agent.Live = true
			agent.LastActiveAt = event.Time
			if agent.State == "" || agent.State == AgentStateUnknown || agent.State == AgentStateGone {
				agent.State = AgentStateRunning
			}
		case EventKindManualNote:
			if agent.State == "" {
				agent.State = AgentStateUnknown
			}
		}
	}

	if agent.State == "" {
		agent.State = AgentStateUnknown
	}

	if agent.State != prevState && !event.Time.IsZero() {
		agent.StateChangedAt = event.Time
	}

	// Track which source last set the state.
	if agent.State != prevState {
		agent.StateSource = event.Source
	}

	if event.Source == EventSourceReconcile {
		agent.ReconcileSource = string(event.Source)
	}

	return agent
}
