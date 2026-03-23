package store

import (
	"context"

	"github.com/mariusae/tmux-agents/internal/model"
)

type Store interface {
	Close() error
	RecordEvent(ctx context.Context, event model.Event) (model.Event, model.Agent, error)
	ListEvents(ctx context.Context, afterSeq uint64, limit int) ([]model.Event, error)
	ListAgents(ctx context.Context) ([]model.Agent, error)
	GetMeta(ctx context.Context, key string) (string, error)
	SetMeta(ctx context.Context, key string, value string) error
}
