package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestStatsHandler(t *testing.T) {
	// 1. Create a KeyPool with 2 keys.
	pool := NewKeyPool([]string{"key-1", "key-2"}, 60)

	// Access the keys directly to set their mock state.
	keys := pool.Keys()
	if len(keys) != 2 {
		t.Fatalf("expected 2 keys, got %d", len(keys))
	}

	keys[0].RemainingRPD = 1000
	keys[0].RemainingTPM = 500
	keys[0].TotalRequests = 10

	keys[1].RemainingRPD = 2000
	keys[1].RemainingTPM = 1500
	keys[1].TotalRequests = 20

	// 2. Create a StatsCollector.
	collector := NewStatsCollector()
	collector.Record("tenant-a")
	collector.Record("tenant-a")
	collector.Record("tenant-b")

	// 3. Create the Handler.
	handler := StatsHandler(pool, collector)

	// 4. Perform request.
	req := httptest.NewRequest("GET", "/stats", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	// 5. Assert status and response structure/values.
	if w.Code != http.StatusOK {
		t.Errorf("expected status OK, got %d", w.Code)
	}

	var resp StatsResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	// Combined RPD: 1000 + 2000 = 3000
	if resp.RemainingRPD != 3000 {
		t.Errorf("expected remaining_rpd to be 3000, got %d", resp.RemainingRPD)
	}

	// Combined TPM: 500 + 1500 = 2000
	if resp.RemainingTPM != 2000 {
		t.Errorf("expected remaining_tpm to be 2000, got %d", resp.RemainingTPM)
	}

	// Combined TotalRequests: 10 + 20 = 30
	if resp.TotalRequests != 30 {
		t.Errorf("expected total_requests to be 30, got %d", resp.TotalRequests)
	}

	// Verify details are intact.
	if len(resp.Keys) != 2 {
		t.Errorf("expected 2 keys in response, got %d", len(resp.Keys))
	}
	if len(resp.Tenants) != 2 {
		t.Errorf("expected 2 tenants in response, got %d", len(resp.Tenants))
	}
}

func TestStatsHandlerDefaultLimits(t *testing.T) {
	pool := NewKeyPool([]string{"key-1", "key-2"}, 60)
	collector := NewStatsCollector()
	handler := StatsHandler(pool, collector)

	req := httptest.NewRequest("GET", "/stats", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status OK, got %d", w.Code)
	}

	var resp StatsResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	// 2 keys * 1,000,000,000 = 2,000,000,000
	if resp.RemainingRPD != 2000000000 {
		t.Errorf("expected default remaining_rpd to be 2000000000, got %d", resp.RemainingRPD)
	}
	if resp.RemainingTPM != 2000000000 {
		t.Errorf("expected default remaining_tpm to be 2000000000, got %d", resp.RemainingTPM)
	}
	if resp.TotalRequests != 0 {
		t.Errorf("expected default total_requests to be 0, got %d", resp.TotalRequests)
	}
}
