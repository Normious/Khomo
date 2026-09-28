package routing

import (
	"context"
	"sync"
	"time"

	"github.com/Normious/Khomo/internal/db"
)

// Registry keeps the router up to date by reloading from DB periodically
// or on-demand (via admin endpoint).
type Registry struct {
	db         *db.DB
	mu         sync.RWMutex
	router     *Router
	lastReload time.Time
}

func NewRegistry(database *db.DB) (*Registry, error) {
	reg := &Registry{db: database}
	if err := reg.Reload(context.Background()); err != nil {
		return nil, err
	}

	// Background reload every 30 seconds
	go func() {
		ticker := time.NewTicker(30 * time.Second)
		defer ticker.Stop()
		for range ticker.C {
			_ = reg.Reload(context.Background())
		}
	}()

	return reg, nil
}

func (reg *Registry) Reload(ctx context.Context) error {
	routes, backends, err := reg.db.LoadAllRoutes(ctx)
	if err != nil {
		return err
	}

	newRouter := NewRouter(routes, backends)

	reg.mu.Lock()
	reg.router = newRouter
	reg.lastReload = time.Now()
	reg.mu.Unlock()

	return nil
}

func (reg *Registry) Get() *Router {
	reg.mu.RLock()
	defer reg.mu.RUnlock()
	return reg.router
}

func (reg *Registry) LastReload() time.Time {
	reg.mu.RLock()
	defer reg.mu.RUnlock()
	return reg.lastReload
}
