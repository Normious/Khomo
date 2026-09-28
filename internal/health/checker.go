package health

import (
	"context"
	"net/http"
	"sync"
	"time"

	"github.com/Normious/Khomo/internal/db"
)

// Checker periodically pings every backend's /health endpoint
// and tracks per-backend health status.
type Checker struct {
	db            *db.DB
	client        *http.Client
	path          string
	mu            sync.RWMutex
	backendHealth map[int64]bool // backendID → healthy
	enabled       bool
}

func NewChecker(database *db.DB, enabled bool, timeoutSecs int, path string) *Checker {
	if timeoutSecs <= 0 {
		timeoutSecs = 5
	}
	if path == "" {
		path = "/health"
	}

	return &Checker{
		db:            database,
		client:        &http.Client{Timeout: time.Duration(timeoutSecs) * time.Second},
		path:          path,
		backendHealth: make(map[int64]bool),
		enabled:       enabled,
	}
}

// Start begins background health checks. Call Stop() to stop.
func (c *Checker) Start(ctx context.Context, intervalSecs int) {
	if !c.enabled {
		return
	}
	if intervalSecs <= 0 {
		intervalSecs = 30
	}

	go func() {
		ticker := time.NewTicker(time.Duration(intervalSecs) * time.Second)
		defer ticker.Stop()

		// Run once immediately
		c.checkAll(ctx)

		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				c.checkAll(ctx)
			}
		}
	}()
}

func (c *Checker) checkAll(ctx context.Context) {
	_, backends, err := c.db.LoadAllRoutes(ctx)
	if err != nil {
		return
	}

	var wg sync.WaitGroup
	for _, list := range backends {
		for _, b := range list {
			wg.Add(1)
			go func(backend *db.Backend) {
				defer wg.Done()
				c.checkOne(ctx, backend)
			}(b)
		}
	}
	wg.Wait()
}

func (c *Checker) checkOne(ctx context.Context, b *db.Backend) {
	url := b.BaseURL + c.path
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		c.set(b.ID, false)
		return
	}
	req.Header.Set("User-Agent", "Khomo-HealthChecker/1.0")

	resp, err := c.client.Do(req)
	if err != nil {
		c.set(b.ID, false)
		return
	}
	defer resp.Body.Close()

	c.set(b.ID, resp.StatusCode >= 200 && resp.StatusCode < 400)
}

func (c *Checker) set(backendID int64, healthy bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.backendHealth[backendID] = healthy
}

// IsHealthy returns the last health status for a backend.
func (c *Checker) IsHealthy(backendID int64) bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	v, ok := c.backendHealth[backendID]
	if !ok {
		return true // Assume healthy until first check
	}
	return v
}

// Snapshot returns the current health map.
func (c *Checker) Snapshot() map[int64]bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	out := make(map[int64]bool, len(c.backendHealth))
	for k, v := range c.backendHealth {
		out[k] = v
	}
	return out
}
