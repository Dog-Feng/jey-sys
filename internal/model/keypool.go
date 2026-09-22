package model

import (
	"log/slog"
	"strings"
	"sync"
)

// KeyPool rotates TypeSafe API keys when quota/billing errors occur.
type KeyPool struct {
	mu   sync.Mutex
	keys []string
	idx  int
	log  *slog.Logger
}

func NewKeyPool(keys []string, start int, log *slog.Logger) *KeyPool {
	if len(keys) == 0 {
		return &KeyPool{log: log}
	}
	if start < 0 || start >= len(keys) {
		start = 0
	}
	return &KeyPool{keys: keys, idx: start, log: log}
}

func (p *KeyPool) Len() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return len(p.keys)
}

func (p *KeyPool) Current() string {
	p.mu.Lock()
	defer p.mu.Unlock()
	if len(p.keys) == 0 {
		return ""
	}
	return p.keys[p.idx]
}

// Rotate advances to the next key. Returns false if only one key or empty.
func (p *KeyPool) Rotate(reason string) bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	if len(p.keys) <= 1 {
		return false
	}
	old := p.idx
	p.idx = (p.idx + 1) % len(p.keys)
	if p.log != nil {
		p.log.Warn("typesafe key rotate",
			"from", old+1,
			"to", p.idx+1,
			"total", len(p.keys),
			"reason", reason,
		)
	}
	return true
}

func isTypeSafeQuotaError(status int, body string) bool {
	if status == 402 || status == 429 {
		return true
	}
	b := strings.ToLower(body)
	for _, s := range []string{
		"insufficient", "quota", "credit", "balance", "payment required",
		"billing", "exceeded", "limit", "out of",
	} {
		if strings.Contains(b, s) {
			return true
		}
	}
	return false
}
