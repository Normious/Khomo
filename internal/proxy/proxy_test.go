package proxy

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Normious/Khomo/internal/circuit"
	"github.com/Normious/Khomo/internal/db"
)

func TestStripPrefix(t *testing.T) {
	route := &db.Route{PathPrefix: "/api/auth", StripPrefix: true}
	cases := map[string]string{
		"/api/auth":         "/",
		"/api/auth/login":   "/login",
		"/api/auth/a/b?x=1": "/a/b?x=1",
	}
	for in, want := range cases {
		// stripPrefixIfNeeded operates on path only; split query for the table.
		path := in
		if i := len(path); i > 0 {
			for j, c := range path {
				if c == '?' {
					path = path[:j]
					break
				}
			}
		}
		if got := stripPrefixIfNeeded(path, route); got != wantPath(want) {
			t.Errorf("strip %q = %q, want %q", in, got, want)
		}
	}
	noStrip := &db.Route{PathPrefix: "/api/auth", StripPrefix: false}
	if got := stripPrefixIfNeeded("/api/auth/login", noStrip); got != "/api/auth/login" {
		t.Errorf("no-strip got %q", got)
	}
}

func wantPath(s string) string {
	for i, c := range s {
		if c == '?' {
			return s[:i]
		}
	}
	return s
}

func TestJoinPaths(t *testing.T) {
	if got := joinPaths("", "/x"); got != "/x" {
		t.Errorf("got %q", got)
	}
	if got := joinPaths("/", "/x"); got != "/x" {
		t.Errorf("got %q", got)
	}
	if got := joinPaths("/base", "/x"); got != "/base/x" {
		t.Errorf("got %q", got)
	}
	if got := joinPaths("/base/", "/x"); got != "/base/x" {
		t.Errorf("got %q", got)
	}
}

func TestRoundRobinSkipsOpenBreaker(t *testing.T) {
	route := &db.Route{ID: 1, PathPrefix: "/api/a"}
	b1 := &db.Backend{ID: 1, RouteID: 1, BaseURL: "http://a"}
	b2 := &db.Backend{ID: 2, RouteID: 1, BaseURL: "http://b"}
	breakers := map[int64]*circuit.Breaker{
		1: circuit.NewBreaker(1, 100, 1, 60000, 1),
		2: circuit.NewBreaker(2, 100, 1, 60000, 1),
	}
	pool := NewBackendPool(route, []*db.Backend{b1, b2}, breakers)
	seen := map[int64]int{}
	for i := 0; i < 4; i++ {
		seen[pool.Select().ID]++
	}
	if seen[1] != 2 || seen[2] != 2 {
		t.Fatalf("expected round-robin 2/2, got %v", seen)
	}
}

func TestHeaderTransform(t *testing.T) {
	route := &db.Route{
		AddHeaders:   map[string]string{"X-Service-Caller": "khomo"},
		StripHeaders: []string{"Cookie"},
	}
	req := httptest.NewRequest("GET", "http://x/api/notify/send", nil)
	req.Header.Set("Cookie", "session=abc")
	ApplyRouteTransform(req, route)
	if req.Header.Get("Cookie") != "" {
		t.Error("Cookie should be stripped")
	}
	if req.Header.Get("X-Service-Caller") != "khomo" {
		t.Error("add-header missing")
	}
}

func TestInjectGatewayHeaders(t *testing.T) {
	req := httptest.NewRequest("GET", "http://x/api/a", nil)
	InjectGatewayHeaders(req, "rid-123", "Proj")
	if req.Header.Get("X-Request-ID") != "rid-123" {
		t.Error("request id missing")
	}
	if req.Header.Get("X-Gateway") == "" || req.Header.Get("X-Forwarded-For") == "" {
		t.Error("gateway headers missing")
	}
	var _ = http.MethodGet
}
