package routing

import (
	"sort"
	"strings"

	"github.com/Normious/Khomo/internal/db"
)

// Router holds routes indexed for fast longest-prefix matching.
type Router struct {
	// Sorted by prefix length descending, so longer prefixes match first.
	routes []*db.Route
	// Map prefix → route for direct lookup
	byPrefix map[string]*db.Route
	// Route ID → backends
	backends map[int64][]*db.Backend
}

func NewRouter(routes map[string]*db.Route, backends map[int64][]*db.Backend) *Router {
	r := &Router{
		routes:   make([]*db.Route, 0, len(routes)),
		byPrefix: make(map[string]*db.Route, len(routes)),
		backends: backends,
	}

	for _, route := range routes {
		r.routes = append(r.routes, route)
		r.byPrefix[route.PathPrefix] = route
	}

	// Sort by prefix length descending (longest matches first)
	sort.Slice(r.routes, func(i, j int) bool {
		return len(r.routes[i].PathPrefix) > len(r.routes[j].PathPrefix)
	})

	return r
}

// Match returns the route whose prefix best matches the given path.
func (r *Router) Match(path string) *db.Route {
	for _, route := range r.routes {
		prefix := route.PathPrefix
		// Exact prefix match: either path == prefix, or path starts with prefix + "/"
		if path == prefix || strings.HasPrefix(path, prefix+"/") {
			return route
		}
	}
	return nil
}

// BackendsFor returns the active backends for a route.
func (r *Router) BackendsFor(route *db.Route) []*db.Backend {
	if route == nil {
		return nil
	}
	return r.backends[route.ID]
}

// Routes returns all routes (for admin/stats endpoints).
func (r *Router) Routes() []*db.Route {
	return r.routes
}
