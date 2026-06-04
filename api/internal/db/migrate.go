package db

import (
	"database/sql"
	"embed"
	"fmt"

	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"
)

const migrationAdvisoryLockID int64 = 0x6169646f6373

//go:embed migrations/*.sql
var migrations embed.FS

// Migrate applies database migrations under a PostgreSQL advisory lock so
// horizontally scaled app instances can safely start at the same time. The
// connection pool is constrained to one connection while migrating so the
// session-scoped advisory lock is held by the same connection used by goose.
func Migrate(databaseURL string) error {
	db, err := sql.Open("pgx", databaseURL)
	if err != nil {
		return err
	}
	defer db.Close()
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)

	if _, err := db.Exec(`SELECT pg_advisory_lock($1)`, migrationAdvisoryLockID); err != nil {
		return fmt.Errorf("acquire migration advisory lock: %w", err)
	}
	defer db.Exec(`SELECT pg_advisory_unlock($1)`, migrationAdvisoryLockID)

	goose.SetBaseFS(migrations)
	return goose.Up(db, "migrations")
}
