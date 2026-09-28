package db

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"time"

	_ "modernc.org/sqlite"
)

type DB struct {
	*sql.DB
}

func Open(path string) (*DB, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return nil, fmt.Errorf("create data dir: %w", err)
	}

	db, err := sql.Open("sqlite",
		path+"?_pragma=journal_mode(WAL)&_pragma=foreign_keys(1)&_pragma=synchronous(NORMAL)")
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}
	db.SetMaxOpenConns(15)
	db.SetMaxIdleConns(5)
	db.SetConnMaxLifetime(time.Hour)

	if err := db.Ping(); err != nil {
		return nil, fmt.Errorf("ping sqlite: %w", err)
	}

	wrapper := &DB{DB: db}
	if err := wrapper.migrate(); err != nil {
		return nil, fmt.Errorf("migrate: %w", err)
	}
	return wrapper, nil
}

func (d *DB) migrate() error {
	candidates := []string{
		filepath.Join("migrations", "0001_init.sql"),
		filepath.Join(".", "migrations", "0001_init.sql"),
		"/migrations/0001_init.sql",
	}
	// Also try relative to the running executable (Docker FROM scratch copies to /migrations).
	if exe, err := os.Executable(); err == nil {
		candidates = append(candidates,
			filepath.Join(filepath.Dir(exe), "migrations", "0001_init.sql"),
		)
	}
	// And try current working directory explicitly.
	if cwd, err := os.Getwd(); err == nil {
		candidates = append(candidates,
			filepath.Join(cwd, "migrations", "0001_init.sql"),
		)
	}

	var sqlBytes []byte
	var err error
	for _, p := range candidates {
		sqlBytes, err = os.ReadFile(p)
		if err == nil {
			break
		}
	}
	if err != nil {
		return fmt.Errorf("read migration (tried %v): %w", candidates, err)
	}
	if _, err := d.Exec(string(sqlBytes)); err != nil {
		return fmt.Errorf("exec migration: %w", err)
	}
	return nil
}
