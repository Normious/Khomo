-- ─────────────────────────────────────────────
-- 1. Projects (Tenants / API Key Holders)
-- ─────────────────────────────────────────────
CREATE TABLE IF NOT EXISTS projects (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    name TEXT NOT NULL,
    api_key TEXT UNIQUE NOT NULL,
    rate_limit_per_minute INTEGER DEFAULT 600,
    daily_request_quota INTEGER DEFAULT 1000000,
    is_active INTEGER DEFAULT 1,
    created_at INTEGER NOT NULL,
    updated_at INTEGER NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_projects_api_key ON projects(api_key);

-- ─────────────────────────────────────────────
-- 2. Routes (path prefix → service)
-- ─────────────────────────────────────────────
CREATE TABLE IF NOT EXISTS routes (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    path_prefix TEXT UNIQUE NOT NULL,
    service_name TEXT NOT NULL,
    strip_prefix INTEGER DEFAULT 1,
    require_auth INTEGER DEFAULT 1,
    timeout_ms INTEGER DEFAULT 30000,
    max_retries INTEGER DEFAULT 2,
    add_headers TEXT DEFAULT '{}',
    strip_headers TEXT DEFAULT '[]',
    websocket INTEGER DEFAULT 0,
    is_active INTEGER DEFAULT 1,
    created_at INTEGER NOT NULL,
    updated_at INTEGER NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_routes_prefix ON routes(path_prefix);
CREATE INDEX IF NOT EXISTS idx_routes_active ON routes(is_active);

-- ─────────────────────────────────────────────
-- 3. Backends (one or more per route)
-- ─────────────────────────────────────────────
CREATE TABLE IF NOT EXISTS backends (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    route_id INTEGER NOT NULL,
    base_url TEXT NOT NULL,
    weight INTEGER DEFAULT 1,
    is_active INTEGER DEFAULT 1,
    created_at INTEGER NOT NULL,
    FOREIGN KEY (route_id) REFERENCES routes(id) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS idx_backends_route ON backends(route_id);

-- ─────────────────────────────────────────────
-- 4. Request Log (for observability, short TTL)
-- ─────────────────────────────────────────────
CREATE TABLE IF NOT EXISTS request_log (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    project_id INTEGER,
    request_id TEXT NOT NULL,
    method TEXT NOT NULL,
    path TEXT NOT NULL,
    route_prefix TEXT,
    service_name TEXT,
    backend_url TEXT,
    status_code INTEGER,
    request_size_bytes INTEGER,
    response_size_bytes INTEGER,
    duration_ms INTEGER,
    backend_duration_ms INTEGER,
    retry_count INTEGER DEFAULT 0,
    circuit_state TEXT,
    cache_hit TEXT,
    user_agent TEXT,
    client_ip TEXT,
    error_message TEXT,
    created_at INTEGER NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_reqlog_project ON request_log(project_id);
CREATE INDEX IF NOT EXISTS idx_reqlog_request_id ON request_log(request_id);
CREATE INDEX IF NOT EXISTS idx_reqlog_route ON request_log(route_prefix);
CREATE INDEX IF NOT EXISTS idx_reqlog_created ON request_log(created_at);
CREATE INDEX IF NOT EXISTS idx_reqlog_status ON request_log(status_code);

-- ─────────────────────────────────────────────
-- 5. Circuit Breaker State (per backend, in-memory mirrored to DB)
-- ─────────────────────────────────────────────
CREATE TABLE IF NOT EXISTS circuit_state (
    backend_id INTEGER PRIMARY KEY,
    state TEXT NOT NULL DEFAULT 'closed',
    consecutive_failures INTEGER DEFAULT 0,
    consecutive_successes INTEGER DEFAULT 0,
    last_failure_at INTEGER,
    opened_at INTEGER,
    updated_at INTEGER NOT NULL,
    FOREIGN KEY (backend_id) REFERENCES backends(id) ON DELETE CASCADE
);

-- ─────────────────────────────────────────────
-- 6. Daily Summary
-- ─────────────────────────────────────────────
CREATE TABLE IF NOT EXISTS daily_summary (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    project_id INTEGER NOT NULL,
    date TEXT NOT NULL,
    total_requests INTEGER DEFAULT 0,
    success_2xx INTEGER DEFAULT 0,
    client_errors_4xx INTEGER DEFAULT 0,
    server_errors_5xx INTEGER DEFAULT 0,
    gateway_errors INTEGER DEFAULT 0,
    avg_duration_ms REAL DEFAULT 0,
    p95_duration_ms INTEGER DEFAULT 0,
    total_bytes_out INTEGER DEFAULT 0,
    retries INTEGER DEFAULT 0,
    rate_limited INTEGER DEFAULT 0,
    updated_at INTEGER NOT NULL,
    UNIQUE(project_id, date)
);

CREATE INDEX IF NOT EXISTS idx_daily_project ON daily_summary(project_id);
CREATE INDEX IF NOT EXISTS idx_daily_date ON daily_summary(date);
