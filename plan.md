# tmux-agents Plan

## Objective

Build `tmux-agents` as a single self-contained Go binary that monitors agent activity across tmux panes, records hook-driven events, reconciles missing state from live tmux/process inspection, and exposes that state through:

- a default TUI selector
- a tmux status-line command
- a log stream
- explicit record and reconcile commands

## Product Scope

### Required commands

```text
tmux-agents
tmux-agents status
tmux-agents record <agent> <session> <fact/event>
tmux-agents log [-f]
tmux-agents reconcile
```

### Required runtime properties

- single static Go binary
- no mandatory background daemon
- persistent local state
- hook-friendly CLI entrypoints
- live reconciliation for providers with incomplete hooks
- agent records include both provider session identity and tmux session/window/pane

## Design Principles

- Hooks are low-latency writers, not the source of truth.
- The local store is authoritative for history and current materialized state.
- Reconciliation repairs missing or stale facts from live tmux and process state.
- `status` must be cheap enough for tmux refresh use.
- The default UI and the status line should read from the same state model.

## Architecture

The binary is split into three layers:

1. Store
2. Reconciler
3. Interfaces

### 1. Store

Use an embedded local store with:

- an append-only event log
- a materialized current-state view
- lightweight metadata for snapshots and previews

Preferred backend:

- `bbolt`

Reasoning:

- pure Go
- static-build friendly
- simple operational model
- supports both sequential event appends and indexed current-state lookups

### 2. Reconciler

The reconciler scans tmux and the local process tree to infer agent presence and state when hooks are missing or incomplete.

It should support two modes:

- bounded reconcile: cheap refresh used by `status` and the TUI
- full reconcile: explicit `tmux-agents reconcile`

### 3. Interfaces

- CLI commands for record/log/status/reconcile
- TUI for agent selection and live preview
- hook entrypoints implemented as normal `record` calls

## State Model

### Agent identity

Each logical agent entry should contain:

- `provider`: `claude` or `codex`
- `provider_session_id`: hook-provided when available, synthetic otherwise
- `tmux_session`
- `tmux_window`
- `tmux_pane`

Identity key for storage:

```text
<provider>:<provider_session_id>
```

If a provider session ID is unavailable, synthesize one from tmux identity and first-seen time. This avoids dropping live-only sessions discovered by reconciliation.

### Event model

Store immutable events with at least:

- `id`
- `time`
- `provider`
- `provider_session_id`
- `tmux_session`
- `tmux_window`
- `tmux_pane`
- `kind`
- `message`
- `source`
- `metadata`

Initial event kinds:

- `prompt_submitted`
- `tool_started`
- `tool_finished`
- `turn_completed`
- `notification`
- `live_detected`
- `live_missing`
- `pane_changed`
- `pane_closed`
- `pane_moved`
- `manual_note`

Event sources:

- `hook`
- `reconcile`
- `user`
- `system`

### Materialized current state

Maintain a current-state record per logical agent:

- identity fields
- `state`
- `awaiting_input`
- `live`
- `last_event_at`
- `last_active_at`
- `last_seen_at`
- `last_preview_at`
- `preview_pane`
- `preview_cache`
- `reconcile_source`

Initial states:

- `running`
- `awaiting_input`
- `idle`
- `gone`
- `unknown`

State rules:

- hooks can directly promote `running` or `awaiting_input`
- reconciliation can mark `running`, `idle`, or `gone`
- absence from tmux should not delete history; it should produce `gone`

## Command Behavior

### `tmux-agents`

Launch the TUI.

Layout:

- left pane: agent inbox ordered by most recent activity
- right pane: live pane preview from tmux capture

Behavior:

- refresh list on a short interval
- refresh preview for the selected agent
- `Enter` navigates to the target pane in tmux

### `tmux-agents status`

Print a compact summary of agents that are awaiting user input.

Behavior:

- run a bounded reconcile if the snapshot is stale
- read the materialized view
- output a short stable format suitable for `#(tmux-agents status)`

Initial output target:

```text
2 waiting: codex/app, claude/docs
```

If no agents are waiting:

```text
no agents waiting
```

### `tmux-agents record <agent> <session> <fact/event>`

Record a structured event associated with an agent session.

Initial CLI contract:

- keep the user-facing command simple
- normalize the final write into a structured event

Behavior:

- accept provider and provider session ID explicitly
- auto-discover tmux session/window/pane from the current environment when possible
- allow free-form message payloads

### `tmux-agents log [-f]`

Read the event log in time order.

Behavior:

- default: print historical events
- `-f`: stream newly appended events

### `tmux-agents reconcile`

Perform a full live scan and write any newly inferred events and state changes.

Behavior:

- discover agent-bearing tmux panes
- infer provider type
- map live panes to existing or synthetic agent identities
- record missing transitions
- mark vanished panes as `gone`

## Reconciliation Strategy

### tmux discovery

Use tmux commands to enumerate panes and metadata:

- `tmux list-panes -a -F ...`
- `tmux display-message -p ...`
- `tmux capture-pane -p -e -J ...`

Fields to collect:

- session name
- window index/name
- pane id
- pane pid
- pane title
- current command
- activity timestamps if available

### provider detection

Detect providers using process names and pane metadata.

Initial heuristics:

- Claude: process tree contains `claude`
- Codex: process tree contains `codex`

### Codex gap-filling

Codex does not expose a full hook stream, so the reconciler must infer activity.

Initial heuristics:

- recent pane output change suggests activity
- recent process-tree change or active subprocesses suggests running work
- known completion notification or hook event suggests `awaiting_input`
- if a previously waiting session becomes active again, flip back to `running`

This should remain heuristic but explicit in code so behavior is testable and replaceable.

### Snapshot staleness

To keep `status` cheap:

- store `last_reconcile_at`
- skip bounded reconcile if the last refresh is recent enough
- use a small threshold such as 1 second for tmux status integration

## Package Layout

Initial package plan:

```text
cmd/tmux-agents/
internal/app/
internal/cli/
internal/store/
internal/model/
internal/reconcile/
internal/tmux/
internal/process/
internal/tui/
internal/logstream/
```

Responsibilities:

- `internal/model`: shared types and state transitions
- `internal/store`: `bbolt` persistence and materialized views
- `internal/tmux`: tmux command wrappers and preview capture
- `internal/process`: process-tree inspection helpers
- `internal/reconcile`: bounded/full reconciliation logic
- `internal/cli`: subcommand parsing and output formatting
- `internal/tui`: selector and live preview
- `internal/app`: top-level wiring

## Milestones

### Milestone 1: project bootstrap

- initialize Go module
- add command skeleton
- define core models
- implement store open/close and basic event append
- add `plan.md` and initial README notes

Exit criteria:

- `tmux-agents --help` works
- events can be written and read locally

### Milestone 2: record/log/status baseline

- implement `record`
- implement `log`
- implement materialized agent view
- implement minimal `status`

Exit criteria:

- hook-like calls can update an agent record
- `status` shows awaiting-input sessions from stored state

### Milestone 3: tmux discovery and full reconcile

- implement tmux pane enumeration
- implement provider detection
- implement state repair and vanished-pane handling
- add bounded reconcile support for `status`

Exit criteria:

- live tmux panes produce or update agent records without hooks
- stale sessions are marked `gone`

### Milestone 4: TUI selector

- inbox-style list ordered by last activity
- live pane preview
- jump-to-pane on enter

Exit criteria:

- `tmux-agents` is useful as a standalone selector

### Milestone 5: hardening

- improve provider heuristics
- optimize status-path latency
- document tmux integration examples
- add integration tests against isolated tmux servers

## Testing Strategy

### Unit tests

- event normalization
- state transition rules
- materialized-view updates
- status formatting
- reconcile merge logic

### Integration tests

- isolated tmux server using `tmux -L <name> -f /dev/null`
- pane enumeration
- pane preview capture
- pane navigation
- bounded reconcile behavior

### Manual test matrix

- Claude with hooks only
- Codex with mixed hook and reconcile tracking
- multiple tmux sessions
- pane moves and pane closes
- stale sessions after process exit
- `status` under tmux `status-interval`

## Open Questions

- exact hook payloads available from Claude and Codex
- best synthetic session ID format for live-only sessions
- whether `status` should show only a count or also short agent labels
- whether pane preview should be raw tmux capture or lightly normalized

## Immediate Next Steps

1. Initialize the Go module and command skeleton.
2. Define the core model types and store interfaces.
3. Implement the persistent event log and current-state materialization.
4. Add `record`, `log`, and a minimal `status` command before the TUI.
