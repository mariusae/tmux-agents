package store

import (
	"context"
	"encoding/binary"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"time"

	bolt "go.etcd.io/bbolt"

	"github.com/mariusae/tmux-agents/internal/model"
)

var (
	eventsBucket = []byte("events")
	agentsBucket = []byte("agents")
	metaBucket   = []byte("meta")
)

type BoltStore struct {
	db       *bolt.DB
	readOnly bool
}

func OpenBolt(path string) (*BoltStore, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, err
	}

	db, err := bolt.Open(path, 0o600, &bolt.Options{Timeout: 5 * time.Second})
	if err != nil {
		return nil, err
	}

	if err := db.Update(func(tx *bolt.Tx) error {
		for _, bucket := range [][]byte{eventsBucket, agentsBucket, metaBucket} {
			if _, err := tx.CreateBucketIfNotExists(bucket); err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		_ = db.Close()
		return nil, err
	}

	return &BoltStore{db: db}, nil
}

func OpenBoltReadOnly(path string) (*BoltStore, error) {
	db, err := bolt.Open(path, 0o600, &bolt.Options{
		Timeout:  5 * time.Second,
		ReadOnly: true,
	})
	if err != nil {
		return nil, err
	}
	return &BoltStore{db: db, readOnly: true}, nil
}

func (s *BoltStore) Close() error {
	return s.db.Close()
}

func (s *BoltStore) RecordEvent(_ context.Context, event model.Event) (model.Event, model.Agent, error) {
	if s.readOnly {
		return model.Event{}, model.Agent{}, errors.New("store is read-only")
	}
	if event.Provider == "" {
		return model.Event{}, model.Agent{}, errors.New("provider is required")
	}
	if event.ProviderSessionID == "" {
		return model.Event{}, model.Agent{}, errors.New("provider session id is required")
	}
	if event.Kind == "" {
		return model.Event{}, model.Agent{}, errors.New("event kind is required")
	}
	if event.Time.IsZero() {
		event.Time = time.Now().UTC()
	}
	if event.Source == "" {
		event.Source = model.EventSourceUser
	}

	var agent model.Agent
	err := s.db.Update(func(tx *bolt.Tx) error {
		eventBucket := tx.Bucket(eventsBucket)
		agentBucket := tx.Bucket(agentsBucket)

		seq, err := eventBucket.NextSequence()
		if err != nil {
			return err
		}
		event.Seq = seq

		key := []byte(event.AgentKey())
		if existing := agentBucket.Get(key); existing != nil {
			if err := json.Unmarshal(existing, &agent); err != nil {
				return err
			}
		}
		agent = model.ApplyEvent(agent, event)

		eventBytes, err := json.Marshal(event)
		if err != nil {
			return err
		}
		agentBytes, err := json.Marshal(agent)
		if err != nil {
			return err
		}

		if err := eventBucket.Put(encodeUint64(seq), eventBytes); err != nil {
			return err
		}
		if err := agentBucket.Put(key, agentBytes); err != nil {
			return err
		}
		return tx.Bucket(metaBucket).Put([]byte("last_event_seq"), []byte(stringifyUint64(seq)))
	})
	if err != nil {
		return model.Event{}, model.Agent{}, err
	}

	return event, agent, nil
}

func (s *BoltStore) ListEvents(_ context.Context, afterSeq uint64, limit int) ([]model.Event, error) {
	events := make([]model.Event, 0)
	err := s.db.View(func(tx *bolt.Tx) error {
		cursor := tx.Bucket(eventsBucket).Cursor()
		start := encodeUint64(afterSeq + 1)
		for key, value := cursor.Seek(start); key != nil; key, value = cursor.Next() {
			var event model.Event
			if err := json.Unmarshal(value, &event); err != nil {
				return err
			}
			events = append(events, event)
			if limit > 0 && len(events) >= limit {
				break
			}
		}
		return nil
	})
	return events, err
}

func (s *BoltStore) ListAgents(_ context.Context) ([]model.Agent, error) {
	agents := make([]model.Agent, 0)
	err := s.db.View(func(tx *bolt.Tx) error {
		return tx.Bucket(agentsBucket).ForEach(func(_, value []byte) error {
			var agent model.Agent
			if err := json.Unmarshal(value, &agent); err != nil {
				return err
			}
			agents = append(agents, agent)
			return nil
		})
	})
	return agents, err
}

func (s *BoltStore) GetMeta(_ context.Context, key string) (string, error) {
	var value string
	err := s.db.View(func(tx *bolt.Tx) error {
		raw := tx.Bucket(metaBucket).Get([]byte(key))
		if raw != nil {
			value = string(raw)
		}
		return nil
	})
	return value, err
}

func (s *BoltStore) SetMeta(_ context.Context, key string, value string) error {
	if s.readOnly {
		return errors.New("store is read-only")
	}
	return s.db.Update(func(tx *bolt.Tx) error {
		return tx.Bucket(metaBucket).Put([]byte(key), []byte(value))
	})
}

func encodeUint64(v uint64) []byte {
	buf := make([]byte, 8)
	binary.BigEndian.PutUint64(buf, v)
	return buf
}

func stringifyUint64(v uint64) string {
	return strconv.FormatUint(v, 10)
}
