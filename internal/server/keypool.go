package server

import (
	"errors"
	"net/http"
	"strconv"
	"sync"
	"sync/atomic"
	"time"
)

// ErrAllKeysExhausted is returned when every key in the pool is on cooldown.
var ErrAllKeysExhausted = errors.New("all API keys are currently rate-limited")

// KeyState tracks the runtime state of a single Cerebras API key.
type KeyState struct {
	APIKey        string
	CooldownUntil time.Time // Zero value means available.
	RemainingRPD  int64     // From x-ratelimit-remaining-requests-day.
	RemainingTPM  int64     // From x-ratelimit-remaining-tokens-minute.
	TotalRequests int64     // Lifetime successful requests.
	TotalErrors   int64     // Lifetime error responses (4xx/5xx from upstream).
	mu            sync.RWMutex
}

// IsAvailable returns true if the key is not on cooldown.
func (k *KeyState) IsAvailable() bool {
	k.mu.RLock()
	defer k.mu.RUnlock()
	return k.CooldownUntil.IsZero() || time.Now().After(k.CooldownUntil)
}

// Status returns a human-readable status string for the key.
func (k *KeyState) Status() string {
	k.mu.RLock()
	defer k.mu.RUnlock()
	if k.CooldownUntil.IsZero() || time.Now().After(k.CooldownUntil) {
		return "available"
	}
	return "cooling_down"
}

// MaskedKey returns the API key with all but the last 4 characters masked.
func (k *KeyState) MaskedKey() string {
	if len(k.APIKey) <= 4 {
		return "****"
	}
	return k.APIKey[:4] + "..." + k.APIKey[len(k.APIKey)-4:]
}

// KeyPool manages a pool of Cerebras API keys with round-robin selection
// and per-key cooldown tracking.
type KeyPool struct {
	keys              []*KeyState
	counter           atomic.Uint64
	defaultCooldownSec int
}

// NewKeyPool creates a key pool from a list of API key strings.
func NewKeyPool(apiKeys []string, defaultCooldownSec int) *KeyPool {
	keys := make([]*KeyState, len(apiKeys))
	for i, k := range apiKeys {
		keys[i] = &KeyState{
			APIKey:       k,
			RemainingRPD: 2400,
			RemainingTPM: 30000,
		}
	}
	return &KeyPool{
		keys:              keys,
		defaultCooldownSec: defaultCooldownSec,
	}
}

// Next selects the next available key using round-robin, skipping keys
// that are currently on cooldown. Returns ErrAllKeysExhausted if every
// key is cooling down.
func (p *KeyPool) Next() (*KeyState, error) {
	n := uint64(len(p.keys))
	start := p.counter.Add(1) % n

	for i := uint64(0); i < n; i++ {
		idx := (start + i) % n
		key := p.keys[idx]
		if key.IsAvailable() {
			return key, nil
		}
	}

	return nil, ErrAllKeysExhausted
}

// MarkCooldown puts a key on cooldown based on response headers from a 429 response.
// If the reset header is missing or unparseable, falls back to defaultCooldownSec.
func (p *KeyPool) MarkCooldown(key *KeyState, headers http.Header) {
	duration := time.Duration(p.defaultCooldownSec) * time.Second

	// Try to parse the reset time from Cerebras rate-limit headers.
	if resetStr := headers.Get("x-ratelimit-reset-tokens-minute"); resetStr != "" {
		if secs, err := strconv.ParseFloat(resetStr, 64); err == nil && secs > 0 {
			duration = time.Duration(secs * float64(time.Second))
		}
	}

	key.mu.Lock()
	key.CooldownUntil = time.Now().Add(duration)
	key.mu.Unlock()

	atomic.AddInt64(&key.TotalErrors, 1)
}

// UpdateState updates a key's rate limit state from successful response headers.
func (p *KeyPool) UpdateState(key *KeyState, headers http.Header) {
	atomic.AddInt64(&key.TotalRequests, 1)

	key.mu.Lock()
	defer key.mu.Unlock()

	if v := headers.Get("x-ratelimit-remaining-requests-day"); v != "" {
		if n, err := strconv.ParseInt(v, 10, 64); err == nil {
			key.RemainingRPD = n
		}
	}

	if v := headers.Get("x-ratelimit-remaining-tokens-minute"); v != "" {
		if n, err := strconv.ParseInt(v, 10, 64); err == nil {
			key.RemainingTPM = n
		}
	}
}

// ShortestCooldown returns the duration until the next key becomes available.
// Used to set the Retry-After header when all keys are exhausted.
func (p *KeyPool) ShortestCooldown() time.Duration {
	now := time.Now()
	shortest := time.Duration(0)

	for _, key := range p.keys {
		key.mu.RLock()
		if !key.CooldownUntil.IsZero() && key.CooldownUntil.After(now) {
			remaining := key.CooldownUntil.Sub(now)
			if shortest == 0 || remaining < shortest {
				shortest = remaining
			}
		}
		key.mu.RUnlock()
	}

	if shortest == 0 {
		shortest = time.Duration(p.defaultCooldownSec) * time.Second
	}

	return shortest
}

// Keys returns the underlying key states (for stats reporting).
func (p *KeyPool) Keys() []*KeyState {
	return p.keys
}

// Len returns the number of keys in the pool.
func (p *KeyPool) Len() int {
	return len(p.keys)
}

// AvailableCount returns how many keys are currently not on cooldown.
func (p *KeyPool) AvailableCount() int {
	count := 0
	for _, key := range p.keys {
		if key.IsAvailable() {
			count++
		}
	}
	return count
}

// CoolingCount returns how many keys are currently on cooldown.
func (p *KeyPool) CoolingCount() int {
	return p.Len() - p.AvailableCount()
}
