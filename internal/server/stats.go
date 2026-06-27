package server

import (
	"encoding/json"
	"net/http"
	"sync"
	"sync/atomic"
	"time"
)

// TenantStats tracks usage for a single tenant.
type TenantStats struct {
	Name          string `json:"name"`
	TotalRequests int64  `json:"total_requests"`
}

// StatsCollector aggregates per-tenant and per-key usage statistics.
type StatsCollector struct {
	mu        sync.RWMutex
	tenants   map[string]*TenantStats
	startTime time.Time
}

// NewStatsCollector creates a new stats collector.
func NewStatsCollector() *StatsCollector {
	return &StatsCollector{
		tenants:   make(map[string]*TenantStats),
		startTime: time.Now(),
	}
}

// Record increments counters for a tenant after a successful proxy response.
func (s *StatsCollector) Record(tenantName string) {
	s.mu.RLock()
	ts, exists := s.tenants[tenantName]
	s.mu.RUnlock()

	if exists {
		atomic.AddInt64(&ts.TotalRequests, 1)
		return
	}

	// Slow path: tenant not yet tracked.
	s.mu.Lock()
	// Double-check under write lock.
	ts, exists = s.tenants[tenantName]
	if !exists {
		ts = &TenantStats{Name: tenantName}
		s.tenants[tenantName] = ts
	}
	s.mu.Unlock()

	atomic.AddInt64(&ts.TotalRequests, 1)
}

// KeyStatsEntry is the JSON representation of a single key's stats.
type KeyStatsEntry struct {
	Index         int        `json:"index"`
	MaskedKey     string     `json:"masked_key"`
	Status        string     `json:"status"`
	CooldownUntil *time.Time `json:"cooldown_until"`
	RemainingRPD  int64      `json:"remaining_rpd"`
	RemainingTPM  int64      `json:"remaining_tpm"`
	TotalRequests int64      `json:"total_requests"`
	TotalErrors   int64      `json:"total_errors"`
}

// StatsResponse is the top-level JSON response for /stats.
type StatsResponse struct {
	Keys          []KeyStatsEntry `json:"keys"`
	Tenants       []TenantStats   `json:"tenants"`
	UptimeSeconds float64         `json:"uptime_seconds"`
}

// StatsHandler returns an http.HandlerFunc that serves the /stats endpoint.
func StatsHandler(pool *KeyPool, collector *StatsCollector) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		keys := pool.Keys()
		keyEntries := make([]KeyStatsEntry, len(keys))

		for i, k := range keys {
			k.mu.RLock()
			entry := KeyStatsEntry{
				Index:         i,
				MaskedKey:     k.MaskedKey(),
				Status:        k.Status(),
				RemainingRPD:  k.RemainingRPD,
				RemainingTPM:  k.RemainingTPM,
				TotalRequests: atomic.LoadInt64(&k.TotalRequests),
				TotalErrors:   atomic.LoadInt64(&k.TotalErrors),
			}
			if !k.CooldownUntil.IsZero() && k.CooldownUntil.After(time.Now()) {
				t := k.CooldownUntil
				entry.CooldownUntil = &t
			}
			k.mu.RUnlock()
			keyEntries[i] = entry
		}

		collector.mu.RLock()
		tenantEntries := make([]TenantStats, 0, len(collector.tenants))
		for _, ts := range collector.tenants {
			tenantEntries = append(tenantEntries, TenantStats{
				Name:          ts.Name,
				TotalRequests: atomic.LoadInt64(&ts.TotalRequests),
			})
		}
		collector.mu.RUnlock()

		resp := StatsResponse{
			Keys:          keyEntries,
			Tenants:       tenantEntries,
			UptimeSeconds: time.Since(collector.startTime).Seconds(),
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}
}

// HealthResponse is the JSON response for /health.
type HealthResponse struct {
	Status       string `json:"status"`
	KeysAvail    int    `json:"keys_available"`
	KeysCooling  int    `json:"keys_cooling"`
}

// HealthHandler returns an http.HandlerFunc that serves the /health endpoint.
func HealthHandler(pool *KeyPool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		resp := HealthResponse{
			Status:      "ok",
			KeysAvail:   pool.AvailableCount(),
			KeysCooling: pool.CoolingCount(),
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}
}
