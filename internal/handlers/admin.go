package handlers

import (
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	"github.com/Normious/Khomo/internal/db"
	"github.com/Normious/Khomo/internal/proxy"
	"github.com/Normious/Khomo/internal/routing"
)

type AdminHandler struct {
	Registry *routing.Registry
	DB       *db.DB
	Px       *proxy.Proxy
	CB       proxy.CBConfig
}

func NewAdminHandler(reg *routing.Registry, database *db.DB, px *proxy.Proxy, cb proxy.CBConfig) *AdminHandler {
	return &AdminHandler{Registry: reg, DB: database, Px: px, CB: cb}
}

// Reload forces the router to reload routes from the database and re-syncs proxy pools.
func (h *AdminHandler) Reload(w http.ResponseWriter, r *http.Request) {
	if err := h.Registry.Reload(r.Context()); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{
			"success": false,
			"error":   err.Error(),
		})
		return
	}
	router := h.Registry.Get()
	h.Px.SyncRoutes(router.Routes(), router.BackendsFor, h.CB)
	writeJSON(w, http.StatusOK, map[string]any{
		"success":     true,
		"reloaded_at": h.Registry.LastReload(),
	})
}

// ListRoutes returns all active routes (for debugging).
func (h *AdminHandler) ListRoutes(w http.ResponseWriter, r *http.Request) {
	router := h.Registry.Get()
	routes := router.Routes()

	type backendInfo struct {
		ID      int64  `json:"id"`
		BaseURL string `json:"base_url"`
		Weight  int    `json:"weight"`
	}
	type routeInfo struct {
		ID          int64         `json:"id"`
		PathPrefix  string        `json:"path_prefix"`
		ServiceName string        `json:"service_name"`
		StripPrefix bool          `json:"strip_prefix"`
		RequireAuth bool          `json:"require_auth"`
		TimeoutMs   int           `json:"timeout_ms"`
		MaxRetries  int           `json:"max_retries"`
		WebSocket   bool          `json:"websocket"`
		Backends    []backendInfo `json:"backend_list"`
		BackendCnt  int           `json:"backends"`
	}

	var out []routeInfo
	for _, rt := range routes {
		bs := router.BackendsFor(rt)
		bi := make([]backendInfo, 0, len(bs))
		for _, b := range bs {
			bi = append(bi, backendInfo{ID: b.ID, BaseURL: b.BaseURL, Weight: b.Weight})
		}
		out = append(out, routeInfo{
			ID: rt.ID, PathPrefix: rt.PathPrefix, ServiceName: rt.ServiceName,
			StripPrefix: rt.StripPrefix, RequireAuth: rt.RequireAuth,
			TimeoutMs: rt.TimeoutMs, MaxRetries: rt.MaxRetries, WebSocket: rt.WebSocket,
			Backends: bi, BackendCnt: len(bi),
		})
	}
	if out == nil {
		out = []routeInfo{}
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"success": true,
		"count":   len(out),
		"routes":  out,
	})
}

// ─── Route CRUD ───────────────────────────────────────────────

type createRouteReq struct {
	PathPrefix   string            `json:"path_prefix"`
	ServiceName  string            `json:"service_name"`
	StripPrefix  *bool             `json:"strip_prefix"`
	RequireAuth  *bool             `json:"require_auth"`
	TimeoutMs    int               `json:"timeout_ms"`
	MaxRetries   int               `json:"max_retries"`
	AddHeaders   map[string]string `json:"add_headers"`
	StripHeaders []string          `json:"strip_headers"`
	WebSocket    bool              `json:"websocket"`
}

func (h *AdminHandler) CreateRoute(w http.ResponseWriter, r *http.Request) {
	var req createRouteReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"success": false, "error": "invalid JSON body"})
		return
	}
	if req.PathPrefix == "" || req.ServiceName == "" {
		writeJSON(w, http.StatusBadRequest, map[string]any{"success": false, "error": "path_prefix and service_name are required"})
		return
	}
	route := &db.Route{
		PathPrefix: req.PathPrefix, ServiceName: req.ServiceName,
		StripPrefix: true, RequireAuth: true,
		TimeoutMs: req.TimeoutMs, MaxRetries: req.MaxRetries,
		AddHeaders: req.AddHeaders, StripHeaders: req.StripHeaders,
		WebSocket: req.WebSocket,
	}
	if req.StripPrefix != nil {
		route.StripPrefix = *req.StripPrefix
	}
	if req.RequireAuth != nil {
		route.RequireAuth = *req.RequireAuth
	}
	if route.TimeoutMs <= 0 {
		route.TimeoutMs = 30000
	}
	if route.AddHeaders == nil {
		route.AddHeaders = map[string]string{}
	}
	id, err := h.DB.CreateRoute(r.Context(), route)
	if err != nil {
		writeJSON(w, http.StatusConflict, map[string]any{"success": false, "error": err.Error()})
		return
	}
	h.Reload(w, r.WithContext(r.Context()))
	_ = id
}

func (h *AdminHandler) DeleteRoute(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id <= 0 {
		writeJSON(w, http.StatusBadRequest, map[string]any{"success": false, "error": "invalid route id"})
		return
	}
	if err := h.DB.DeleteRoute(r.Context(), id); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"success": false, "error": err.Error()})
		return
	}
	h.Reload(w, r.WithContext(r.Context()))
}

// ─── Backend CRUD ─────────────────────────────────────────────

type createBackendReq struct {
	RouteID int64  `json:"route_id"`
	BaseURL string `json:"base_url"`
	Weight  int    `json:"weight"`
}

func (h *AdminHandler) CreateBackend(w http.ResponseWriter, r *http.Request) {
	var req createBackendReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"success": false, "error": "invalid JSON body"})
		return
	}
	if req.RouteID <= 0 || req.BaseURL == "" {
		writeJSON(w, http.StatusBadRequest, map[string]any{"success": false, "error": "route_id and base_url are required"})
		return
	}
	if req.Weight <= 0 {
		req.Weight = 1
	}
	if _, err := h.DB.CreateBackend(r.Context(), req.RouteID, req.BaseURL, req.Weight); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"success": false, "error": err.Error()})
		return
	}
	h.Reload(w, r.WithContext(r.Context()))
}

func (h *AdminHandler) DeleteBackend(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id <= 0 {
		writeJSON(w, http.StatusBadRequest, map[string]any{"success": false, "error": "invalid backend id"})
		return
	}
	if err := h.DB.DeleteBackend(r.Context(), id); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"success": false, "error": err.Error()})
		return
	}
	h.Reload(w, r.WithContext(r.Context()))
}

// ─── Project CRUD (minimal) ───────────────────────────────────

type createProjectReq struct {
	Name               string `json:"name"`
	APIKey             string `json:"api_key"`
	RateLimitPerMinute int    `json:"rate_limit_per_minute"`
}

func (h *AdminHandler) CreateProject(w http.ResponseWriter, r *http.Request) {
	var req createProjectReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"success": false, "error": "invalid JSON body"})
		return
	}
	if req.Name == "" || req.APIKey == "" {
		writeJSON(w, http.StatusBadRequest, map[string]any{"success": false, "error": "name and api_key are required"})
		return
	}
	if req.RateLimitPerMinute <= 0 {
		req.RateLimitPerMinute = 600
	}
	id, err := h.DB.CreateProject(r.Context(), req.Name, req.APIKey, req.RateLimitPerMinute)
	if err != nil {
		writeJSON(w, http.StatusConflict, map[string]any{"success": false, "error": err.Error()})
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"success": true, "id": id})
}

func (h *AdminHandler) ListProjects(w http.ResponseWriter, r *http.Request) {
	projects, err := h.DB.ListProjects(r.Context())
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"success": false, "error": err.Error()})
		return
	}
	type info struct {
		ID                 int64  `json:"id"`
		Name               string `json:"name"`
		RateLimitPerMinute int    `json:"rate_limit_per_minute"`
		IsActive           bool   `json:"is_active"`
	}
	out := make([]info, 0, len(projects))
	for _, p := range projects {
		out = append(out, info{ID: p.ID, Name: p.Name, RateLimitPerMinute: p.RateLimitPerMinute, IsActive: p.IsActive})
	}
	writeJSON(w, http.StatusOK, map[string]any{"success": true, "count": len(out), "projects": out, "at": time.Now().UTC()})
}
