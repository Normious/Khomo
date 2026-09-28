package routing

import (
	"testing"

	"github.com/Normious/Khomo/internal/db"
)

func mkRoute(id int64, prefix string) *db.Route {
	return &db.Route{ID: id, PathPrefix: prefix, ServiceName: "svc", IsActive: true}
}

func TestLongestPrefixWins(t *testing.T) {
	routes := map[string]*db.Route{
		"/api/auth":       mkRoute(1, "/api/auth"),
		"/api/auth/admin": mkRoute(2, "/api/auth/admin"),
		"/api":            mkRoute(3, "/api"),
	}
	r := NewRouter(routes, nil)

	if got := r.Match("/api/auth/admin/users"); got == nil || got.PathPrefix != "/api/auth/admin" {
		t.Fatalf("expected /api/auth/admin, got %+v", got)
	}
	if got := r.Match("/api/auth/login"); got == nil || got.PathPrefix != "/api/auth" {
		t.Fatalf("expected /api/auth, got %+v", got)
	}
	if got := r.Match("/api/other"); got == nil || got.PathPrefix != "/api" {
		t.Fatalf("expected /api, got %+v", got)
	}
	if got := r.Match("/nope"); got != nil {
		t.Fatalf("expected nil, got %+v", got)
	}
	// Prefix boundary: /api/authx must NOT match /api/auth
	if got := r.Match("/api/authx"); got == nil || got.PathPrefix != "/api" {
		t.Fatalf("expected /api for /api/authx, got %+v", got)
	}
}

func TestExactMatch(t *testing.T) {
	routes := map[string]*db.Route{"/api/auth": mkRoute(1, "/api/auth")}
	r := NewRouter(routes, nil)
	if got := r.Match("/api/auth"); got == nil {
		t.Fatal("expected exact match")
	}
}
