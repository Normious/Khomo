package proxy

import (
	"net/http"
	"strings"

	"github.com/Normious/Khomo/internal/db"
)

// InjectGatewayHeaders adds X-Gateway-* and X-Forwarded-* headers.
// Called on the outbound request before proxying.
func InjectGatewayHeaders(req *http.Request, requestID, projectName string) {
	req.Header.Set("X-Request-ID", requestID)
	req.Header.Set("X-Gateway", "Khomo/1.0")
	req.Header.Set("X-Forwarded-Host", req.Host)
	if req.TLS != nil {
		req.Header.Set("X-Forwarded-Proto", "https")
	} else if req.Header.Get("X-Forwarded-Proto") == "" {
		req.Header.Set("X-Forwarded-Proto", "http")
	}

	// Client IP chain
	ip := clientIP(req)
	existing := req.Header.Get("X-Forwarded-For")
	if existing != "" {
		// Avoid duplicating the IP if it's already the tail of the chain.
		if !strings.Contains(existing, ip) {
			req.Header.Set("X-Forwarded-For", existing+", "+ip)
		}
	} else {
		req.Header.Set("X-Forwarded-For", ip)
	}

	if projectName != "" {
		req.Header.Set("X-Project", projectName)
	}

	// Hop-by-hop headers should not be forwarded
	req.Header.Del("Connection")
	req.Header.Del("Keep-Alive")
	req.Header.Del("Proxy-Authenticate")
	req.Header.Del("Proxy-Authorization")
	req.Header.Del("Te")
	req.Header.Del("Trailer")
	req.Header.Del("Transfer-Encoding")
	req.Header.Del("Upgrade")
}

// ApplyRouteTransform applies per-route header add/strip rules.
func ApplyRouteTransform(req *http.Request, route *db.Route) {
	if route == nil {
		return
	}
	// Add headers
	for k, v := range route.AddHeaders {
		req.Header.Set(k, v)
	}

	// Strip headers
	for _, h := range route.StripHeaders {
		req.Header.Del(h)
	}
}

func clientIP(r *http.Request) string {
	if cf := r.Header.Get("CF-Connecting-IP"); cf != "" {
		return cf
	}
	if fwd := r.Header.Get("X-Forwarded-For"); fwd != "" {
		parts := strings.Split(fwd, ",")
		return strings.TrimSpace(parts[0])
	}
	// RemoteAddr includes port — strip it for a clean IP.
	host := r.RemoteAddr
	if i := strings.LastIndex(host, ":"); i != -1 {
		host = host[:i]
	}
	host = strings.Trim(host, "[]")
	if host == "" {
		return r.RemoteAddr
	}
	return host
}
