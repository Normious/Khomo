package handlers

import (
	"net/http"
	"time"

	"github.com/Normious/Khomo/internal/health"
	"github.com/Normious/Khomo/internal/routing"
)

type HealthHandler struct {
	Registry *routing.Registry
	Checker  *health.Checker
}

func NewHealthHandler(reg *routing.Registry, checker *health.Checker) *HealthHandler {
	return &HealthHandler{Registry: reg, Checker: checker}
}

func (h *HealthHandler) Handle(w http.ResponseWriter, r *http.Request) {
	router := h.Registry.Get()
	routes := router.Routes()

	// Count total backends + health status
	totalBackends := 0
	healthyBackends := 0

	for _, rt := range routes {
		for _, b := range router.BackendsFor(rt) {
			totalBackends++
			if h.Checker.IsHealthy(b.ID) {
				healthyBackends++
			}
		}
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"service":          "Khomo — API Gateway",
		"version":          "1.0.0",
		"language":         "Go",
		"status":           "ok",
		"routes":           len(routes),
		"backends_total":   totalBackends,
		"backends_healthy": healthyBackends,
		"last_reload":      h.Registry.LastReload(),
		"timestamp":        time.Now().UTC(),
	})
}
