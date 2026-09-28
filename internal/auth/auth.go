package auth

import (
	"errors"
	"net/http"

	"github.com/Normious/Khomo/internal/db"
)

var (
	ErrMissingAPIKey = errors.New("missing X-API-Key header")
	ErrInvalidAPIKey = errors.New("invalid or inactive API Key")
)

func Authenticate(r *http.Request, database *db.DB) (*db.Project, error) {
	apiKey := r.Header.Get("X-API-Key")
	if apiKey == "" {
		return nil, ErrMissingAPIKey
	}
	project, err := database.GetProjectByAPIKey(r.Context(), apiKey)
	if err != nil {
		return nil, ErrInvalidAPIKey
	}
	return project, nil
}
