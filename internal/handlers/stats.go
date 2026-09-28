package handlers

import (
	"net/http"
	"strconv"

	"github.com/Normious/Khomo/internal/auth"
	"github.com/Normious/Khomo/internal/db"
)

type StatsHandler struct {
	DB *db.DB
}

func NewStatsHandler(database *db.DB) *StatsHandler {
	return &StatsHandler{DB: database}
}

func (h *StatsHandler) Handle(w http.ResponseWriter, r *http.Request) {
	project, err := auth.Authenticate(r, h.DB)
	if err != nil {
		writeJSON(w, http.StatusUnauthorized, map[string]any{
			"success": false,
			"error":   err.Error(),
		})
		return
	}

	days, _ := strconv.Atoi(r.URL.Query().Get("days"))
	if days <= 0 || days > 365 {
		days = 30
	}

	stats, err := h.DB.GetStats(r.Context(), project.ID, days)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{
			"success": false,
			"error":   "failed to fetch stats",
		})
		return
	}

	stats["success"] = true
	stats["project"] = project.Name
	writeJSON(w, http.StatusOK, stats)
}
