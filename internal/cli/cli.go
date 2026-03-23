package cli

import (
	"context"
	"flag"
	"fmt"
	"io"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/mariusae/tmux-agents/internal/app"
	"github.com/mariusae/tmux-agents/internal/hook"
	"github.com/mariusae/tmux-agents/internal/model"
	"github.com/mariusae/tmux-agents/internal/setup"
	"github.com/mariusae/tmux-agents/internal/tui"
)

func Run(ctx context.Context, args []string, stdout io.Writer, stderr io.Writer) int {
	if len(args) == 0 {
		application, err := app.OpenDefaultReadOnly()
		if err != nil {
			_, _ = fmt.Fprintf(stderr, "open store: %v\n", err)
			return 1
		}
		defer application.Close()

		if err := tui.Run(ctx, application); err != nil && err != context.Canceled {
			_, _ = fmt.Fprintf(stderr, "tui: %v\n", err)
			return 1
		}
		return 0
	}

	switch args[0] {
	case "help", "-h", "--help":
		printUsage(stdout)
		return 0
	case "-rage":
		if err := tui.Rage(ctx, stdout); err != nil {
			_, _ = fmt.Fprintf(stderr, "rage: %v\n", err)
			return 1
		}
		return 0
	case "setup":
		return runSetup(ctx, stdout, stderr)
	case "show-setup":
		return runShowSetup(ctx, stdout, stderr)
	case "install-hooks":
		return runInstallHooks(ctx, stdout, stderr)
	case "uninstall-hooks":
		return runUninstallHooks(ctx, stdout, stderr)
	}

	switch args[0] {
	case "status":
		application, err := app.OpenDefault()
		if err != nil {
			_, _ = fmt.Fprintf(stderr, "open store: %v\n", err)
			return 1
		}
		defer application.Close()
		return runStatus(ctx, application, args[1:], stdout, stderr)
	case "show":
		application, err := app.OpenDefaultReadOnly()
		if err != nil {
			_, _ = fmt.Fprintf(stderr, "open store: %v\n", err)
			return 1
		}
		defer application.Close()
		return runShow(ctx, application, stdout, stderr)
	case "hook":
		application, err := app.OpenDefault()
		if err != nil {
			_, _ = fmt.Fprintf(stderr, "open store: %v\n", err)
			return 1
		}
		defer application.Close()
		return runHook(ctx, application, args[1:], stdout, stderr)
	case "record":
		application, err := app.OpenDefault()
		if err != nil {
			_, _ = fmt.Fprintf(stderr, "open store: %v\n", err)
			return 1
		}
		defer application.Close()
		return runRecord(ctx, application, args[1:], stdout, stderr)
	case "log":
		application, err := app.OpenDefaultReadOnly()
		if err != nil {
			_, _ = fmt.Fprintf(stderr, "open store: %v\n", err)
			return 1
		}
		defer application.Close()
		return runLog(ctx, application, args[1:], stdout, stderr)
	case "reconcile":
		result, err := app.ReconcileDefault(ctx)
		if err != nil {
			_, _ = fmt.Fprintf(stderr, "reconcile: %v\n", err)
			return 1
		}
		_, _ = fmt.Fprintf(stdout, "reconciled %d live agents, updated %d, marked %d missing\n", result.Seen, result.Updated, result.Missing)
		return 0
	default:
		_, _ = fmt.Fprintf(stderr, "unknown command %q\n\n", args[0])
		printUsage(stderr)
		return 1
	}
}

func runStatus(ctx context.Context, application *app.App, args []string, stdout io.Writer, stderr io.Writer) int {
	statusFlags := flag.NewFlagSet("status", flag.ContinueOnError)
	statusFlags.SetOutput(stderr)
	delimiter := statusFlags.String("d", "", "delimiter to append after non-empty status output")
	if err := statusFlags.Parse(args); err != nil {
		return 1
	}

	line, err := application.StatusLine(ctx)
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "status: %v\n", err)
		return 1
	}
	if line != "" && *delimiter != "" {
		line += *delimiter
	}
	_, _ = fmt.Fprintln(stdout, line)
	return 0
}

func runRecord(ctx context.Context, application *app.App, args []string, stdout io.Writer, stderr io.Writer) int {
	recordFlags := flag.NewFlagSet("record", flag.ContinueOnError)
	recordFlags.SetOutput(stderr)
	source := recordFlags.String("source", string(model.EventSourceUser), "event source")
	if err := recordFlags.Parse(args); err != nil {
		return 1
	}

	rest := recordFlags.Args()
	if len(rest) < 3 {
		_, _ = fmt.Fprintln(stderr, "usage: tmux-agents record [--source user|hook|reconcile|system] <agent> <session> <kind> [message...]")
		return 1
	}

	message := ""
	if len(rest) > 3 {
		message = strings.Join(rest[3:], " ")
	}

	event, agent, err := application.Record(ctx, app.RecordRequest{
		Provider:          rest[0],
		ProviderSessionID: rest[1],
		Kind:              rest[2],
		Message:           message,
		Source:            *source,
	})
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "record: %v\n", err)
		return 1
	}

	_, _ = fmt.Fprintf(stdout, "recorded event %d for %s -> %s\n", event.Seq, agent.Label(), agent.State)
	return 0
}

func runHook(ctx context.Context, application *app.App, args []string, stdout io.Writer, stderr io.Writer) int {
	hookFlags := flag.NewFlagSet("hook", flag.ContinueOnError)
	hookFlags.SetOutput(stderr)
	sessionID := hookFlags.String("session", "", "provider session id")
	if err := hookFlags.Parse(args); err != nil {
		return 1
	}

	rest := hookFlags.Args()
	if len(rest) < 2 {
		_, _ = fmt.Fprintln(stderr, "usage: tmux-agents hook [--session id] <claude|codex> <event> [message...]")
		return 1
	}

	message := ""
	if len(rest) > 2 {
		message = strings.Join(rest[2:], " ")
	}

	resolved, err := hook.Resolve(rest[0], rest[1], *sessionID, message)
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "hook: %v\n", err)
		return 1
	}

	event, agent, err := application.Record(ctx, app.RecordRequest{
		Provider:          resolved.Provider,
		ProviderSessionID: resolved.ProviderSessionID,
		Kind:              resolved.Kind,
		Message:           resolved.Message,
		Source:            string(model.EventSourceHook),
	})
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "hook: %v\n", err)
		return 1
	}

	_, _ = fmt.Fprintf(stdout, "recorded hook event %d for %s -> %s\n", event.Seq, agent.Label(), agent.State)
	return 0
}

func runLog(ctx context.Context, application *app.App, args []string, stdout io.Writer, stderr io.Writer) int {
	logFlags := flag.NewFlagSet("log", flag.ContinueOnError)
	logFlags.SetOutput(stderr)
	follow := logFlags.Bool("f", false, "follow appended events")
	limit := logFlags.Int("n", 0, "maximum events to print")
	if err := logFlags.Parse(args); err != nil {
		return 1
	}

	lastSeq := uint64(0)
	for {
		events, err := application.ListEvents(ctx, lastSeq, *limit)
		if err != nil {
			_, _ = fmt.Fprintf(stderr, "log: %v\n", err)
			return 1
		}
		for _, event := range events {
			_, _ = fmt.Fprintln(stdout, formatEvent(event))
			lastSeq = event.Seq
		}

		if !*follow {
			return 0
		}

		select {
		case <-ctx.Done():
			return 0
		case <-time.After(1 * time.Second):
		}
	}
}

func runShow(ctx context.Context, application *app.App, stdout io.Writer, stderr io.Writer) int {
	agents, err := application.AgentsSnapshot(ctx)
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "show: %v\n", err)
		return 1
	}

	if len(agents) == 0 {
		_, _ = fmt.Fprintln(stdout, "no agents")
		return 0
	}

	table := tabwriter.NewWriter(stdout, 0, 0, 2, ' ', 0)
	_, _ = fmt.Fprintln(table, "ACTIVE\tSTATE\tLIVE\tAGENT")
	for _, agent := range agents {
		live := "no"
		if agent.Live {
			live = "yes"
		}
		_, _ = fmt.Fprintf(table, "%s\t%s\t%s\t%s\n", formatShowTime(time.Now(), agent.LastActivityAt()), agent.State, live, agent.Label())
	}
	_ = table.Flush()
	return 0
}

func runShowSetup(ctx context.Context, stdout io.Writer, stderr io.Writer) int {
	text, err := setup.ShowSetupText(ctx)
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "show-setup: %v\n", err)
		return 1
	}
	_, _ = fmt.Fprintln(stdout, text)
	return 0
}

func runInstallHooks(ctx context.Context, stdout io.Writer, stderr io.Writer) int {
	report, err := setup.InstallHooks(ctx)
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "install-hooks: %v\n", err)
		return 1
	}
	printHookReport(stdout, report)
	return 0
}

func runUninstallHooks(ctx context.Context, stdout io.Writer, stderr io.Writer) int {
	report, err := setup.UninstallHooks(ctx)
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "uninstall-hooks: %v\n", err)
		return 1
	}
	printHookReport(stdout, report)
	return 0
}

func runSetup(ctx context.Context, stdout io.Writer, stderr io.Writer) int {
	messages, err := setup.ApplyTmuxSetup(ctx)
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "setup: %v\n", err)
		return 1
	}

	report, err := setup.InstallHooks(ctx)
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "setup: %v\n", err)
		return 1
	}

	for _, message := range messages {
		_, _ = fmt.Fprintln(stdout, message)
	}
	printHookReport(stdout, report)
	return 0
}

func formatEvent(event model.Event) string {
	agentLabel := event.Provider
	if target := eventTargetLabel(event); target != "" {
		agentLabel = fmt.Sprintf("%s@%s", event.Provider, target)
	} else if strings.TrimSpace(event.ProviderSessionID) != "" {
		agentLabel = fmt.Sprintf("%s/%s", event.Provider, event.ProviderSessionID)
	}

	message := ""
	if event.Message != "" {
		message = " " + event.Message
	}

	return fmt.Sprintf(
		"%06d %s %s %s%s",
		event.Seq,
		event.Time.Format(time.RFC3339),
		agentLabel,
		event.Kind,
		message,
	)
}

func eventTargetLabel(event model.Event) string {
	session := strings.TrimSpace(event.TmuxSession)
	window := event.TmuxWindow
	if head, _, found := strings.Cut(strings.TrimSpace(event.TmuxWindow), ":"); found {
		window = head
	}
	pane := strings.TrimSpace(event.TmuxPane)
	if session == "" || window == "" || pane == "" || strings.HasPrefix(pane, "%") {
		return ""
	}
	return fmt.Sprintf("%s:%s.%s", session, window, pane)
}

func formatShowTime(now, t time.Time) string {
	if t.IsZero() {
		return "-"
	}

	now = now.Local()
	t = t.Local()
	if t.After(now) {
		t = now
	}

	age := now.Sub(t)
	switch {
	case age < 30*time.Second:
		return "just now"
	case age < 90*time.Second:
		return "last minute"
	case age < time.Hour:
		return fmt.Sprintf("%dmin", int(age/time.Minute))
	case age < 24*time.Hour:
		return fmt.Sprintf("%dh", int(age/time.Hour))
	}

	nowY, nowM, nowD := now.Date()
	tY, tM, tD := t.Date()
	nowDate := time.Date(nowY, nowM, nowD, 0, 0, 0, 0, now.Location())
	tDate := time.Date(tY, tM, tD, 0, 0, 0, 0, t.Location())

	if nowDate.Sub(tDate) == 24*time.Hour {
		return "yesterday"
	}
	if now.Year() == t.Year() {
		return t.Format("Mon02")
	}
	return t.Format("02Jan06")
}

func printRootPlaceholder(w io.Writer) {
	_, _ = fmt.Fprintln(w, "tui not implemented yet")
	_, _ = fmt.Fprintln(w, "use: tmux-agents status | show | show-setup | setup | install-hooks | uninstall-hooks | hook | record | log | reconcile")
}

func printUsage(w io.Writer) {
	_, _ = fmt.Fprintln(w, "tmux-agents")
	_, _ = fmt.Fprintln(w)
	_, _ = fmt.Fprintln(w, "Commands:")
	_, _ = fmt.Fprintln(w, "  -rage")
	_, _ = fmt.Fprintln(w, "  setup")
	_, _ = fmt.Fprintln(w, "  status [-d delimiter]")
	_, _ = fmt.Fprintln(w, "  show")
	_, _ = fmt.Fprintln(w, "  show-setup")
	_, _ = fmt.Fprintln(w, "  install-hooks")
	_, _ = fmt.Fprintln(w, "  uninstall-hooks")
	_, _ = fmt.Fprintln(w, "  hook [--session id] <claude|codex> <event> [message...]")
	_, _ = fmt.Fprintln(w, "  record [--source user|hook|reconcile|system] <agent> <session> <kind> [message...]")
	_, _ = fmt.Fprintln(w, "  log [-f] [-n count]")
	_, _ = fmt.Fprintln(w, "  reconcile")
}

func printHookReport(w io.Writer, report setup.HookReport) {
	printed := false
	for _, change := range report.Changes {
		if !change.Changed || change.Diff == "" {
			continue
		}
		if printed {
			_, _ = fmt.Fprintln(w)
		}
		_, _ = fmt.Fprintln(w, change.Diff)
		printed = true
	}
	if !printed {
		_, _ = fmt.Fprintln(w, "no changes")
	}
}
