package server

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strconv"
)

// CerebrasTransport implements http.RoundTripper and handles key rotation
// with retry on 429 responses.
type CerebrasTransport struct {
	Pool     *KeyPool
	Upstream *url.URL
	Base     http.RoundTripper
	Stats    *StatsCollector
}

// RoundTrip implements http.RoundTripper.
func (t *CerebrasTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	// Rewrite URL to upstream.
	req.URL.Scheme = t.Upstream.Scheme
	req.URL.Host = t.Upstream.Host
	req.Host = t.Upstream.Host

	tenant := TenantFromContext(req.Context())
	maxRetries := t.Pool.Len()

	for attempt := 0; attempt < maxRetries; attempt++ {
		key, err := t.Pool.Next()
		if err != nil {
			return t.synthetic429()
		}

		// Clone the request to avoid mutating the original.
		clone := req.Clone(req.Context())
		clone.Header.Set("Authorization", "Bearer "+key.APIKey)

		resp, err := t.Base.RoundTrip(clone)
		if err != nil {
			// Network error — don't retry with a different key.
			return nil, err
		}

		if resp.StatusCode == http.StatusTooManyRequests {
			log.Printf("[cerebro] 429 from upstream on key %s (attempt %d/%d), rotating",
				key.MaskedKey(), attempt+1, maxRetries)
			t.Pool.MarkCooldown(key, resp.Header)
			resp.Body.Close()
			continue
		}

		// Success or non-429 error — update state and return.
		t.Pool.UpdateState(key, resp.Header)
		if tenant != "" {
			t.Stats.Record(tenant)
		}

		return resp, nil
	}

	log.Printf("[cerebro] all %d keys exhausted, returning 429 to client", maxRetries)
	return t.synthetic429()
}

// synthetic429 creates a synthetic 429 response with proper JSON body and Retry-After header.
func (t *CerebrasTransport) synthetic429() (*http.Response, error) {
	cooldown := t.Pool.ShortestCooldown()
	retryAfter := int(cooldown.Seconds()) + 1

	body := apiError{
		Error: apiErrorDetail{
			Message: fmt.Sprintf("All API keys are currently rate-limited. Retry after %d seconds.", retryAfter),
			Type:    "rate_limit_error",
			Code:    "rate_limit_exceeded",
		},
	}
	bodyBytes, _ := json.Marshal(body)

	return &http.Response{
		StatusCode: http.StatusTooManyRequests,
		Header: http.Header{
			"Content-Type": {"application/json"},
			"Retry-After":  {strconv.Itoa(retryAfter)},
		},
		Body:          io.NopCloser(bytes.NewReader(bodyBytes)),
		ContentLength: int64(len(bodyBytes)),
	}, nil
}

// NewProxyHandler creates a configured httputil.ReverseProxy with the custom
// CerebrasTransport for key rotation.
func NewProxyHandler(upstream *url.URL, pool *KeyPool, stats *StatsCollector) http.Handler {
	transport := &CerebrasTransport{
		Pool:     pool,
		Upstream: upstream,
		Base:     http.DefaultTransport,
		Stats:    stats,
	}

	proxy := &httputil.ReverseProxy{
		Rewrite: func(r *httputil.ProxyRequest) {
			r.SetXForwarded()
		},
		Transport:     transport,
		FlushInterval: -1, // Flush immediately for SSE/streaming.
		ModifyResponse: func(resp *http.Response) error {
			log.Printf("[cerebro] %s %s -> %d",
				resp.Request.Method, resp.Request.URL.Path, resp.StatusCode)
			return nil
		},
		ErrorHandler: func(w http.ResponseWriter, r *http.Request, err error) {
			log.Printf("[cerebro] proxy error: %v", err)
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusBadGateway)
			json.NewEncoder(w).Encode(apiError{
				Error: apiErrorDetail{
					Message: "Upstream connection error",
					Type:    "api_error",
					Code:    "upstream_error",
				},
			})
		},
	}

	return proxy
}
