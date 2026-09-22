package store

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"

	"github.com/jev-sys/bot/internal/domain"
)

type JSONL struct {
	path string
	mu   sync.Mutex
}

func NewJSONL(dir string) (*JSONL, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	return &JSONL{path: filepath.Join(dir, "events.jsonl")}, nil
}

func (j *JSONL) Append(ev domain.TickEvent) error {
	j.mu.Lock()
	defer j.mu.Unlock()
	f, err := os.OpenFile(j.path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()
	b, err := json.Marshal(ev)
	if err != nil {
		return err
	}
	_, err = f.Write(append(b, '\n'))
	return err
}
