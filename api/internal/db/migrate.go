package db

import (
	"crypto/sha256"
	"database/sql"
	"embed"
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"

	// Register the pgx database/sql driver used by goose migrations.
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
func Migrate(databaseURL string) (retErr error) {
	db, err := sql.Open("pgx", databaseURL)
	if err != nil {
		return err
	}
	defer func() {
		retErr = errors.Join(retErr, db.Close())
	}()
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)

	if _, err := db.Exec(`SELECT pg_advisory_lock($1)`, migrationAdvisoryLockID); err != nil {
		return fmt.Errorf("acquire migration advisory lock: %w", err)
	}
	defer func() {
		_, unlockErr := db.Exec(`SELECT pg_advisory_unlock($1)`, migrationAdvisoryLockID)
		retErr = errors.Join(retErr, unlockErr)
	}()

	goose.SetBaseFS(migrations)
	return goose.Up(db, "migrations")
}

// Fingerprint returns a short stable hash of the embedded migrations. Test
// infrastructure uses it to cache one migrated template database per schema
// version, so cloning a fresh database is cheap.
func Fingerprint() string {
	h := sha256.New()
	entries, err := fs.Glob(migrations, "migrations/*.sql")
	if err != nil {
		panic(err) // static embed FS: cannot fail with a valid pattern
	}
	for _, name := range entries {
		b, err := fs.ReadFile(migrations, name)
		if err != nil {
			panic(err)
		}
		if _, err := fmt.Fprintf(h, "%s\n", name); err != nil {
			panic(err) // hash.Hash writes cannot fail
		}
		if _, err := h.Write(b); err != nil {
			panic(err) // hash.Hash writes cannot fail
		}
	}
	return hex.EncodeToString(h.Sum(nil))[:10]
}
