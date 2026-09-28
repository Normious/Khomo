package db

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"time"
)

// ─── Types ────────────────────────────────────────────────────

type Project struct {
	ID                 int64
	Name               string
	APIKey             string
	RateLimitPerMinute int
	DailyRequestQuota  int64
	IsActive           bool
}

type Route struct {
	ID           int64
	PathPrefix   string
	ServiceName  string
	StripPrefix  bool
	RequireAuth  bool
	TimeoutMs    int
	MaxRetries   int
	AddHeaders   map[string]string
	StripHeaders []string
	WebSocket    bool
	IsActive     bool
}

type Backend struct {
	ID       int64
	RouteID  int64
	BaseURL  string
	Weight   int
	IsActive bool
}

type RequestLog struct {
	RequestID         string
	ProjectID         int64
	Method            string
	Path              string
	RoutePrefix       string
	ServiceName       string
	BackendURL        string
	StatusCode        int
	RequestSizeBytes  int64
	ResponseSizeBytes int64
	DurationMs        int64
	BackendDurationMs int64
	RetryCount        int
	CircuitState      string
	UserAgent         string
	ClientIP          string
	ErrorMessage      string
}

var ErrNotFound = errors.New("not found")

// ─── Projects ─────────────────────────────────────────────────

func (d *DB) GetProjectByAPIKey(ctx context.Context, apiKey string) (*Project, error) {
	row := d.QueryRowContext(ctx,
		`SELECT id, name, api_key, rate_limit_per_minute, daily_request_quota, is_active
		 FROM projects WHERE api_key = ? AND is_active = 1`, apiKey)

	var p Project
	var active int
	if err := row.Scan(&p.ID, &p.Name, &p.APIKey, &p.RateLimitPerMinute, &p.DailyRequestQuota, &active); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	p.IsActive = active == 1
	return &p, nil
}

func (d *DB) CreateProject(ctx context.Context, name, apiKey string, rateLimit int) (int64, error) {
	now := time.Now().UnixMilli()
	res, err := d.ExecContext(ctx,
		`INSERT INTO projects (name, api_key, rate_limit_per_minute, is_active, created_at, updated_at)
		 VALUES (?, ?, ?, 1, ?, ?)`, name, apiKey, rateLimit, now, now)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

func (d *DB) ListProjects(ctx context.Context) ([]*Project, error) {
	rows, err := d.QueryContext(ctx,
		`SELECT id, name, api_key, rate_limit_per_minute, daily_request_quota, is_active FROM projects ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*Project
	for rows.Next() {
		var p Project
		var active int
		if err := rows.Scan(&p.ID, &p.Name, &p.APIKey, &p.RateLimitPerMinute, &p.DailyRequestQuota, &active); err != nil {
			return nil, err
		}
		p.IsActive = active == 1
		out = append(out, &p)
	}
	return out, rows.Err()
}

// ─── Routes + Backends ────────────────────────────────────────

func (d *DB) LoadAllRoutes(ctx context.Context) (map[string]*Route, map[int64][]*Backend, error) {
	routes := make(map[string]*Route)
	backends := make(map[int64][]*Backend)

	rows, err := d.QueryContext(ctx,
		`SELECT id, path_prefix, service_name, strip_prefix, require_auth,
				timeout_ms, max_retries, add_headers, strip_headers, websocket, is_active
		 FROM routes WHERE is_active = 1`)
	if err != nil {
		return nil, nil, err
	}
	defer rows.Close()

	for rows.Next() {
		var r Route
		var stripPrefix, requireAuth, ws, active int
		var addHeadersJSON, stripHeadersJSON string

		if err := rows.Scan(&r.ID, &r.PathPrefix, &r.ServiceName,
			&stripPrefix, &requireAuth, &r.TimeoutMs, &r.MaxRetries,
			&addHeadersJSON, &stripHeadersJSON, &ws, &active); err != nil {
			return nil, nil, err
		}

		r.StripPrefix = stripPrefix == 1
		r.RequireAuth = requireAuth == 1
		r.WebSocket = ws == 1
		r.IsActive = active == 1

		r.AddHeaders = make(map[string]string)
		if addHeadersJSON != "" {
			_ = json.Unmarshal([]byte(addHeadersJSON), &r.AddHeaders)
		}
		if stripHeadersJSON != "" {
			_ = json.Unmarshal([]byte(stripHeadersJSON), &r.StripHeaders)
		}

		routes[r.PathPrefix] = &r
	}
	if err := rows.Err(); err != nil {
		return nil, nil, err
	}

	rows2, err := d.QueryContext(ctx,
		`SELECT id, route_id, base_url, weight, is_active
		 FROM backends WHERE is_active = 1`)
	if err != nil {
		return nil, nil, err
	}
	defer rows2.Close()

	for rows2.Next() {
		var b Backend
		var active int
		if err := rows2.Scan(&b.ID, &b.RouteID, &b.BaseURL, &b.Weight, &active); err != nil {
			return nil, nil, err
		}
		b.IsActive = active == 1
		backends[b.RouteID] = append(backends[b.RouteID], &b)
	}
	if err := rows2.Err(); err != nil {
		return nil, nil, err
	}

	return routes, backends, nil
}

func boolInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

func (d *DB) CreateRoute(ctx context.Context, r *Route) (int64, error) {
	addH, _ := json.Marshal(r.AddHeaders)
	stripH, _ := json.Marshal(r.StripHeaders)
	if stripH == nil {
		stripH = []byte("[]")
	}
	now := time.Now().UnixMilli()
	res, err := d.ExecContext(ctx,
		`INSERT INTO routes (path_prefix, service_name, strip_prefix, require_auth, timeout_ms, max_retries, add_headers, strip_headers, websocket, is_active, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, 1, ?, ?)`,
		r.PathPrefix, r.ServiceName, boolInt(r.StripPrefix), boolInt(r.RequireAuth),
		r.TimeoutMs, r.MaxRetries, string(addH), string(stripH), boolInt(r.WebSocket), now, now)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

func (d *DB) DeleteRoute(ctx context.Context, id int64) error {
	_, err := d.ExecContext(ctx, `DELETE FROM routes WHERE id = ?`, id)
	return err
}

func (d *DB) CreateBackend(ctx context.Context, routeID int64, baseURL string, weight int) (int64, error) {
	now := time.Now().UnixMilli()
	res, err := d.ExecContext(ctx,
		`INSERT INTO backends (route_id, base_url, weight, is_active, created_at) VALUES (?, ?, ?, 1, ?)`,
		routeID, baseURL, weight, now)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

func (d *DB) DeleteBackend(ctx context.Context, id int64) error {
	_, err := d.ExecContext(ctx, `DELETE FROM backends WHERE id = ?`, id)
	return err
}

// ─── Request Logging ──────────────────────────────────────────

func (d *DB) LogRequest(ctx context.Context, log *RequestLog) error {
	now := time.Now().UnixMilli()

	_, err := d.ExecContext(ctx,
		`INSERT INTO request_log
		 (project_id, request_id, method, path, route_prefix, service_name, backend_url,
		  status_code, request_size_bytes, response_size_bytes,
		  duration_ms, backend_duration_ms, retry_count, circuit_state,
		  user_agent, client_ip, error_message, created_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		log.ProjectID, log.RequestID, log.Method, log.Path,
		log.RoutePrefix, log.ServiceName, log.BackendURL,
		log.StatusCode, log.RequestSizeBytes, log.ResponseSizeBytes,
		log.DurationMs, log.BackendDurationMs, log.RetryCount, log.CircuitState,
		log.UserAgent, log.ClientIP, log.ErrorMessage, now)
	if err != nil {
		return err
	}

	// Skip daily rollup for anonymous traffic (no project yet — e.g. 401/404 before auth).
	// request_log still keeps the full audit trail.
	if log.ProjectID == 0 {
		return nil
	}

	date := time.Now().UTC().Format("2006-01-02")
	is2xx, is4xx, is5xx, isGateway := 0, 0, 0, 0
	switch {
	case log.StatusCode >= 200 && log.StatusCode < 300:
		is2xx = 1
	case log.StatusCode >= 400 && log.StatusCode < 500:
		is4xx = 1
	case log.StatusCode >= 500:
		is5xx = 1
	}
	if log.StatusCode == 502 || log.StatusCode == 503 || log.StatusCode == 504 {
		isGateway = 1
	}

	_, err = d.ExecContext(ctx,
		`INSERT INTO daily_summary
		 (project_id, date, total_requests, success_2xx, client_errors_4xx, server_errors_5xx,
		  gateway_errors, avg_duration_ms, p95_duration_ms, total_bytes_out, retries,
		  rate_limited, updated_at)
		 VALUES (?, ?, 1, ?, ?, ?, ?, ?, 0, ?, ?, 0, ?)
		 ON CONFLICT(project_id, date) DO UPDATE SET
			 total_requests = total_requests + 1,
			 success_2xx = success_2xx + excluded.success_2xx,
			 client_errors_4xx = client_errors_4xx + excluded.client_errors_4xx,
			 server_errors_5xx = server_errors_5xx + excluded.server_errors_5xx,
			 gateway_errors = gateway_errors + excluded.gateway_errors,
			 avg_duration_ms = ((avg_duration_ms * total_requests) + excluded.avg_duration_ms) / (total_requests + 1),
			 total_bytes_out = total_bytes_out + excluded.total_bytes_out,
			 retries = retries + excluded.retries,
			 updated_at = excluded.updated_at`,
		log.ProjectID, date, is2xx, is4xx, is5xx, isGateway,
		float64(log.DurationMs), log.ResponseSizeBytes, log.RetryCount, now)

	return err
}

// ─── Circuit State (persistence for observability) ────────────

func (d *DB) PersistCircuitState(ctx context.Context, backendID int64, state string, failures, successes int) error {
	now := time.Now().UnixMilli()
	_, err := d.ExecContext(ctx,
		`INSERT INTO circuit_state
		 (backend_id, state, consecutive_failures, consecutive_successes, updated_at)
		 VALUES (?, ?, ?, ?, ?)
		 ON CONFLICT(backend_id) DO UPDATE SET
			 state = excluded.state,
			 consecutive_failures = excluded.consecutive_failures,
			 consecutive_successes = excluded.consecutive_successes,
			 updated_at = excluded.updated_at`,
		backendID, state, failures, successes, now)
	return err
}

// ─── Stats ────────────────────────────────────────────────────

func (d *DB) GetStats(ctx context.Context, projectID int64, days int) (map[string]any, error) {
	fromDate := time.Now().UTC().AddDate(0, 0, -days).Format("2006-01-02")

	rows, err := d.QueryContext(ctx,
		`SELECT date, total_requests, success_2xx, client_errors_4xx, server_errors_5xx,
				gateway_errors, avg_duration_ms, total_bytes_out, retries
		 FROM daily_summary WHERE project_id = ? AND date >= ?
		 ORDER BY date ASC`, projectID, fromDate)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var daily []map[string]any
	var totalReq, total2xx, total4xx, total5xx, totalGW, totalBytes, totalRetries int64
	var totalDuration float64

	for rows.Next() {
		var date string
		var req, s2, s4, s5, gw, bytes, retries int64
		var avgDur float64
		if err := rows.Scan(&date, &req, &s2, &s4, &s5, &gw, &avgDur, &bytes, &retries); err != nil {
			return nil, err
		}
		daily = append(daily, map[string]any{
			"date":              date,
			"total_requests":    req,
			"success_2xx":       s2,
			"client_errors_4xx": s4,
			"server_errors_5xx": s5,
			"gateway_errors":    gw,
			"avg_duration_ms":   avgDur,
			"total_bytes_out":   bytes,
			"retries":           retries,
		})
		totalReq += req
		total2xx += s2
		total4xx += s4
		total5xx += s5
		totalGW += gw
		totalBytes += bytes
		totalRetries += retries
		totalDuration += avgDur * float64(req)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if daily == nil {
		daily = []map[string]any{}
	}

	avgAll := 0.0
	if totalReq > 0 {
		avgAll = totalDuration / float64(totalReq)
	}

	routeRows, err := d.QueryContext(ctx,
		`SELECT route_prefix, COUNT(*) as c, AVG(duration_ms) as avg_dur
		 FROM request_log WHERE project_id = ? AND created_at >= ?
		 GROUP BY route_prefix ORDER BY c DESC LIMIT 20`, projectID, time.Now().AddDate(0, 0, -days).UnixMilli())
	if err != nil {
		return nil, err
	}
	defer routeRows.Close()

	topRoutes := make(map[string]map[string]any)
	for routeRows.Next() {
		var prefix sql.NullString
		var count int64
		var avgDur sql.NullFloat64
		if err := routeRows.Scan(&prefix, &count, &avgDur); err != nil {
			return nil, err
		}
		key := ""
		if prefix.Valid {
			key = prefix.String
		}
		avg := 0.0
		if avgDur.Valid {
			avg = avgDur.Float64
		}
		topRoutes[key] = map[string]any{
			"count":           count,
			"avg_duration_ms": avg,
		}
	}
	if err := routeRows.Err(); err != nil {
		return nil, err
	}

	return map[string]any{
		"days":  days,
		"daily": daily,
		"totals": map[string]any{
			"total_requests":    totalReq,
			"success_2xx":       total2xx,
			"client_errors_4xx": total4xx,
			"server_errors_5xx": total5xx,
			"gateway_errors":    totalGW,
			"avg_duration_ms":   avgAll,
			"total_bytes_out":   totalBytes,
			"total_retries":     totalRetries,
		},
		"top_routes": topRoutes,
	}, nil
}
