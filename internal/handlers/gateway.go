package handlers

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/Normious/Khomo/internal/auth"
	"github.com/Normious/Khomo/internal/config"
	"github.com/Normious/Khomo/internal/db"
	"github.com/Normious/Khomo/internal/observability"
	"github.com/Normious/Khomo/internal/proxy"
	"github.com/Normious/Khomo/internal/ratelimit"
	"github.com/Normious/Khomo/internal/routing"
)

type GatewayHandler struct {
	DB       *db.DB
	Config   *config.Config
	Registry *routing.Registry
	Proxy    *proxy.Proxy
	Limiter  *ratelimit.Limiter
	Obs      *observability.Logger
}

func NewGatewayHandler(database *db.DB, cfg *config.Config, registry *routing.Registry, px *proxy.Proxy) *GatewayHandler {
	return &GatewayHandler{
		DB:       database,
		Config:   cfg,
		Registry: registry,
		Proxy:    px,
		Limiter:  ratelimit.NewLimiter(),
		Obs:      observability.NewLogger(database, cfg.LogSampleRate, 2000),
	}
}

// ServeHTTP is the main gateway entry point.
func (h *GatewayHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	start := time.Now()

	// Enforce max request size early (10MB default).
	if h.Config.MaxRequestSizeBytes > 0 && r.ContentLength > h.Config.MaxRequestSizeBytes {
		writeJSON(w, http.StatusRequestEntityTooLarge, map[string]any{
			"success": false,
			"error":   "request too large",
		})
		return
	}
	if h.Config.MaxRequestSizeBytes > 0 {
		r.Body = http.MaxBytesReader(w, r.Body, h.Config.MaxRequestSizeBytes)
	}

	// 1. Request ID
	requestID := r.Header.Get("X-Request-ID")
	if requestID == "" {
		requestID = generateRequestID()
	}
	w.Header().Set("X-Request-ID", requestID)
	r.Header.Set("X-Request-ID", requestID)

	// 2. Match route
	router := h.Registry.Get()
	route := router.Match(r.URL.Path)

	if route == nil {
		writeJSON(w, http.StatusNotFound, map[string]any{
			"success":    false,
			"error":      "no route matched",
			"request_id": requestID,
			"path":       r.URL.Path,
		})
		h.logRequest(r, nil, 0, &proxy.ProxyResult{StatusCode: http.StatusNotFound, ErrorMessage: "no route matched"}, requestID, start)
		return
	}

	// 3. Auth (if required)
	var project *db.Project
	var projectID int64
	if route.RequireAuth {
		p, err := auth.Authenticate(r, h.DB)
		if err != nil {
			writeJSON(w, http.StatusUnauthorized, map[string]any{
				"success":    false,
				"error":      err.Error(),
				"request_id": requestID,
			})
			h.logRequest(r, route, 0, &proxy.ProxyResult{StatusCode: http.StatusUnauthorized, ErrorMessage: err.Error()}, requestID, start)
			return
		}
		project = p
		projectID = p.ID

		// 4. Rate limiting
		if !h.Limiter.Allow(rateLimitKey(project.ID, route.PathPrefix), project.RateLimitPerMinute) {
			writeJSON(w, http.StatusTooManyRequests, map[string]any{
				"success":          false,
				"error":            "rate limit exceeded",
				"request_id":       requestID,
				"limit_per_minute": project.RateLimitPerMinute,
			})
			h.logRequest(r, route, projectID, &proxy.ProxyResult{StatusCode: http.StatusTooManyRequests, ErrorMessage: "rate limit exceeded"}, requestID, start)
			return
		}
	} else if apiKey := r.Header.Get("X-API-Key"); apiKey != "" {
		// Public route but caller supplied a key — attribute traffic when valid.
		if p, err := auth.Authenticate(r, h.DB); err == nil {
			project = p
			projectID = p.ID
		}
	}

	// 5. Inject gateway headers
	proxy.InjectGatewayHeaders(r, requestID, projectName(project))

	// 6. Apply route transforms (add/strip headers)
	proxy.ApplyRouteTransform(r, route)

	// 7. Proxy the request
	result := h.Proxy.ProxyRequest(w, r, route)

	// 8. Log (async, sampled)
	h.logRequest(r, route, projectID, result, requestID, start)
}

func (h *GatewayHandler) logRequest(r *http.Request, route *db.Route, projectID int64, result *proxy.ProxyResult, requestID string, start time.Time) {
	reqLog := &db.RequestLog{
		RequestID:         requestID,
		ProjectID:         projectID,
		Method:            r.Method,
		Path:              r.URL.Path, // path only — query string excluded (PII redaction)
		StatusCode:        result.StatusCode,
		RequestSizeBytes:  max0(r.ContentLength),
		ResponseSizeBytes: result.ResponseSizeBytes,
		DurationMs:        time.Since(start).Milliseconds(),
		BackendDurationMs: result.BackendDurationMs,
		RetryCount:        result.Retries,
		CircuitState:      result.CircuitState,
		UserAgent:         truncate(r.Header.Get("User-Agent"), 256),
		ClientIP:          clientIP(r),
		ErrorMessage:      result.ErrorMessage,
	}

	if route != nil {
		reqLog.RoutePrefix = route.PathPrefix
		reqLog.ServiceName = route.ServiceName
	}
	reqLog.BackendURL = result.BackendURL

	if h.Obs != nil {
		h.Obs.Log(reqLog)
		return
	}
	// Fallback: fire-and-forget direct write.
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		_ = h.DB.LogRequest(ctx, reqLog)
	}()
}

// ─── Helpers ──────────────────────────────────────────────────

func generateRequestID() string {
	var b [12]byte
	_, _ = rand.Read(b[:])
	return hex.EncodeToString(b[:])
}

func rateLimitKey(projectID int64, routePrefix string) string {
	return strings.Join([]string{itoa(projectID), routePrefix}, ":")
}

func projectName(p *db.Project) string {
	if p == nil {
		return ""
	}
	return p.Name
}

func clientIP(r *http.Request) string {
	if cf := r.Header.Get("CF-Connecting-IP"); cf != "" {
		return cf
	}
	if fwd := r.Header.Get("X-Forwarded-For"); fwd != "" {
		parts := strings.Split(fwd, ",")
		return strings.TrimSpace(parts[0])
	}
	return r.RemoteAddr
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}

func itoa(i int64) string {
	if i == 0 {
		return "0"
	}
	neg := i < 0
	if neg {
		i = -i
	}
	var buf [20]byte
	pos := len(buf)
	for i > 0 {
		pos--
		buf[pos] = byte('0' + i%10)
		i /= 10
	}
	if neg {
		pos--
		buf[pos] = '-'
	}
	return string(buf[pos:])
}

func max0(n int64) int64 {
	if n < 0 {
		return 0
	}
	return n
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n]
}
