package proxy

import (
	"bufio"
	"context"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/Normious/Khomo/internal/circuit"
	"github.com/Normious/Khomo/internal/db"
)

// Proxy wraps httputil.ReverseProxy with route-aware behavior.
type Proxy struct {
	db           *db.DB
	pools        map[int64]*BackendPool // routeID → pool
	poolsMu      sync.RWMutex
	preserveHost bool
}

func NewProxy(database *db.DB) *Proxy {
	return &Proxy{
		db:    database,
		pools: make(map[int64]*BackendPool),
	}
}

// RegisterRoute creates a BackendPool for a route with circuit breakers.
func (p *Proxy) RegisterRoute(route *db.Route, backends []*db.Backend, cbConfig CBConfig) {
	breakers := make(map[int64]*circuit.Breaker, len(backends))
	for _, b := range backends {
		breakers[b.ID] = circuit.NewBreaker(
			b.ID,
			cbConfig.FailureThreshold,
			cbConfig.SuccessThreshold,
			cbConfig.Timeout,
			cbConfig.HalfOpenMax,
		)
	}

	pool := NewBackendPool(route, backends, breakers)

	p.poolsMu.Lock()
	p.pools[route.ID] = pool
	p.poolsMu.Unlock()
}

// SyncRoutes rebuilds all pools from the current registry snapshot.
// Call after Registry.Reload so new/removed routes and backends take effect
// without a restart. Existing circuit-breaker states are preserved where the
// backend ID still exists.
func (p *Proxy) SyncRoutes(routes []*db.Route, backendsFor func(*db.Route) []*db.Backend, cbConfig CBConfig) {
	p.poolsMu.Lock()
	defer p.poolsMu.Unlock()

	seen := make(map[int64]bool, len(routes))
	for _, rt := range routes {
		seen[rt.ID] = true
		bs := backendsFor(rt)
		if existing, ok := p.pools[rt.ID]; ok {
			// Refresh backends in place, preserving breakers for known IDs.
			existing.mu.Lock()
			existing.route = rt
			existing.backends = bs
			for _, b := range bs {
				if _, ok := existing.breakers[b.ID]; !ok {
					existing.breakers[b.ID] = circuit.NewBreaker(
						b.ID, cbConfig.FailureThreshold, cbConfig.SuccessThreshold,
						cbConfig.Timeout, cbConfig.HalfOpenMax,
					)
				}
			}
			existing.mu.Unlock()
			continue
		}
		breakers := make(map[int64]*circuit.Breaker, len(bs))
		for _, b := range bs {
			breakers[b.ID] = circuit.NewBreaker(
				b.ID, cbConfig.FailureThreshold, cbConfig.SuccessThreshold,
				cbConfig.Timeout, cbConfig.HalfOpenMax,
			)
		}
		p.pools[rt.ID] = NewBackendPool(rt, bs, breakers)
	}
	for id := range p.pools {
		if !seen[id] {
			delete(p.pools, id)
		}
	}
}

type CBConfig struct {
	FailureThreshold int
	SuccessThreshold int
	Timeout          time.Duration
	HalfOpenMax      int
}

func (p *Proxy) PoolFor(routeID int64) *BackendPool {
	p.poolsMu.RLock()
	defer p.poolsMu.RUnlock()
	return p.pools[routeID]
}

// ProxyResult holds metadata about the proxied request.
type ProxyResult struct {
	StatusCode        int
	BackendURL        string
	BackendDurationMs int64
	ResponseSizeBytes int64
	Retries           int
	CircuitState      string
	ErrorMessage      string
}

// ProxyRequest is the main entry point. It handles retries, circuit breaking,
// and returns structured metadata about the attempt.
func (p *Proxy) ProxyRequest(w http.ResponseWriter, r *http.Request, route *db.Route) *ProxyResult {
	pool := p.PoolFor(route.ID)
	if pool == nil {
		http.Error(w, `{"success":false,"error":"no backend pool registered"}`, http.StatusServiceUnavailable)
		return &ProxyResult{
			StatusCode:   http.StatusServiceUnavailable,
			ErrorMessage: "no backend pool registered",
		}
	}

	maxAttempts := route.MaxRetries + 1 // First attempt + retries
	if maxAttempts < 1 {
		maxAttempts = 1
	}
	var lastResult *ProxyResult

	for attempt := 0; attempt < maxAttempts; attempt++ {
		backend := pool.Select()
		if backend == nil {
			// All backends are circuit-open
			if lastResult == nil {
				lastResult = &ProxyResult{
					StatusCode:   http.StatusServiceUnavailable,
					CircuitState: string(circuit.StateOpen),
					ErrorMessage: "all backends circuit-open",
					Retries:      attempt,
				}
				http.Error(w, `{"success":false,"error":"all backends circuit-open"}`, http.StatusServiceUnavailable)
			}
			break
		}

		result := p.attemptProxy(w, r, route, backend, pool)
		result.Retries = attempt

		// Success (2xx, 3xx) or client error (4xx) → don't retry.
		// The response has already been written to w by the reverse proxy.
		if result.StatusCode < 500 {
			return result
		}

		// 5xx → try next backend (only if attempts remain).
		lastResult = result
	}

	if lastResult == nil {
		lastResult = &ProxyResult{StatusCode: http.StatusBadGateway, ErrorMessage: "no attempts made"}
		http.Error(w, `{"success":false,"error":"bad gateway"}`, http.StatusBadGateway)
	}
	return lastResult
}

func (p *Proxy) attemptProxy(w http.ResponseWriter, r *http.Request, route *db.Route, backend *db.Backend, pool *BackendPool) *ProxyResult {
	start := time.Now()

	// Parse backend URL
	target, err := url.Parse(backend.BaseURL)
	if err != nil {
		pool.RecordFailure(backend)
		http.Error(w, `{"success":false,"error":"invalid backend URL"}`, http.StatusInternalServerError)
		return &ProxyResult{
			StatusCode:   http.StatusInternalServerError,
			ErrorMessage: "invalid backend URL: " + err.Error(),
			BackendURL:   backend.BaseURL,
		}
	}

	preserveHost := p.preserveHost

	// Build reverse proxy. httputil.ReverseProxy handles WebSocket Upgrade
	// via hijacking internally, and our statusRecorder implements Hijack so
	// /ws/* pass-through works.
	proxy := &httputil.ReverseProxy{
		Director: func(req *http.Request) {
			req.URL.Scheme = target.Scheme
			req.URL.Host = target.Host
			req.URL.Path = joinPaths(target.Path, stripPrefixIfNeeded(req.URL.Path, route))
			// RawQuery is preserved as-is (already on req.URL).
			if !preserveHost {
				req.Host = target.Host
			}
		},
		ErrorHandler: func(w http.ResponseWriter, req *http.Request, err error) {
			// Mark the failure so the recorder reflects a 502 for retry logic.
			if rec, ok := w.(*statusRecorder); ok {
				rec.status = http.StatusBadGateway
				rec.wrote = true
			}
			http.Error(w, `{"success":false,"error":"bad gateway"}`, http.StatusBadGateway)
		},
		ModifyResponse: func(resp *http.Response) error {
			// Add X-Gateway-* response headers
			resp.Header.Set("X-Gateway", "Khomo/1.0")
			resp.Header.Set("X-Backend", target.Host)
			return nil
		},
		Transport: &http.Transport{
			MaxIdleConns:        100,
			MaxIdleConnsPerHost: 20,
			IdleConnTimeout:     90 * time.Second,
			DisableCompression:  false,
			ForceAttemptHTTP2:   true,
		},
		FlushInterval: 100 * time.Millisecond, // For streaming responses
	}

	// Store response status via wrapper
	recorder := &statusRecorder{ResponseWriter: w, status: http.StatusOK}

	// Timeout context
	timeout := time.Duration(route.TimeoutMs) * time.Millisecond
	if timeout <= 0 {
		timeout = 30 * time.Second
	}
	timeoutCtx, cancel := context.WithTimeout(r.Context(), timeout)
	defer cancel()

	// Serve
	proxy.ServeHTTP(recorder, r.WithContext(timeoutCtx))

	duration := time.Since(start).Milliseconds()

	// Circuit breaker state update
	var state circuit.State
	if br := pool.BreakerFor(backend.ID); br != nil {
		state, _, _ = br.Stats()
	}
	if recorder.status >= 500 {
		pool.RecordFailure(backend)
	} else {
		pool.RecordSuccess(backend)
	}

	return &ProxyResult{
		StatusCode:        recorder.status,
		BackendURL:        backend.BaseURL,
		BackendDurationMs: duration,
		ResponseSizeBytes: recorder.bytesWritten,
		CircuitState:      string(state),
	}
}

// statusRecorder captures the response status code + bytes written.
type statusRecorder struct {
	http.ResponseWriter
	status       int
	bytesWritten int64
	wrote        bool
}

func (r *statusRecorder) WriteHeader(code int) {
	if !r.wrote {
		r.status = code
		r.wrote = true
	}
	r.ResponseWriter.WriteHeader(code)
}

func (r *statusRecorder) Write(b []byte) (int, error) {
	if !r.wrote {
		r.wrote = true
	}
	n, err := r.ResponseWriter.Write(b)
	r.bytesWritten += int64(n)
	return n, err
}

// Hijack for WebSocket support
func (r *statusRecorder) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	hj, ok := r.ResponseWriter.(http.Hijacker)
	if !ok {
		return nil, nil, http.ErrNotSupported
	}
	return hj.Hijack()
}

// ─── Helpers ──────────────────────────────────────────────────

func stripPrefixIfNeeded(path string, route *db.Route) string {
	if route == nil || !route.StripPrefix {
		return path
	}
	stripped := strings.TrimPrefix(path, route.PathPrefix)
	if stripped == "" {
		return "/"
	}
	if !strings.HasPrefix(stripped, "/") {
		stripped = "/" + stripped
	}
	return stripped
}

func joinPaths(base, path string) string {
	if base == "" || base == "/" {
		return path
	}
	if strings.HasSuffix(base, "/") {
		base = base[:len(base)-1]
	}
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}
	return base + path
}

// Set configuration
func (p *Proxy) SetPreserveHost(preserve bool) {
	p.poolsMu.Lock()
	defer p.poolsMu.Unlock()
	p.preserveHost = preserve
}
