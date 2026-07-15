package db

import (
	"database/sql"
	"fmt"
	"net/url"
	"os"
	"testing"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"
)

func TestGoogleGroupMigrationUpDownAndConstraints(t *testing.T) {
	databaseURL := os.Getenv("AIDOCS_TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("set AIDOCS_TEST_DATABASE_URL to run postgres migration tests")
	}
	parsed, err := url.Parse(databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	databaseName := fmt.Sprintf("aidocs_migration_%d", time.Now().UnixNano())
	adminURL := *parsed
	adminURL.Path = "/postgres"
	admin, err := sql.Open("pgx", adminURL.String())
	if err != nil {
		t.Fatal(err)
	}
	defer admin.Close()
	if _, err := admin.Exec(`CREATE DATABASE "` + databaseName + `"`); err != nil {
		t.Fatal(err)
	}
	defer admin.Exec(`DROP DATABASE IF EXISTS "` + databaseName + `" WITH (FORCE)`)

	testURL := *parsed
	testURL.Path = "/" + databaseName
	database, err := sql.Open("pgx", testURL.String())
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	goose.SetBaseFS(migrations)
	if err := goose.Up(database, "migrations"); err != nil {
		t.Fatal(err)
	}
	for _, table := range []string{"google_groups", "google_group_memberships", "google_group_sync_state"} {
		var exists bool
		if err := database.QueryRow(`SELECT to_regclass($1) IS NOT NULL`, table).Scan(&exists); err != nil || !exists {
			t.Fatalf("table %s exists=%v err=%v", table, exists, err)
		}
	}
	for _, values := range []string{
		`('not-a-resource','C123','group@example.com','Bad',now())`,
		`('groups/has/slash','C123','group@example.com','Bad',now())`,
		`('groups/valid','external','group@example.com','Bad',now())`,
		`('groups/valid','C123','not-an-email','Bad',now())`,
	} {
		if _, err := database.Exec(`
			INSERT INTO google_groups(id,customer_id,email,display_name,last_verified_at)
			VALUES ` + values); err == nil {
			t.Fatalf("google_groups accepted invalid values %s", values)
		}
	}
	if _, err := database.Exec(`
		INSERT INTO google_groups(id,customer_id,email,display_name,last_verified_at)
		VALUES('groups/one','C123','group@example.com','Group',now())`); err != nil {
		t.Fatal(err)
	}
	if _, err := database.Exec(`
		INSERT INTO google_groups(id,customer_id,email,display_name,last_verified_at)
		VALUES('groups/two','C123','GROUP@example.com','Duplicate',now())`); err == nil {
		t.Fatal("google_groups accepted a case-insensitive duplicate email")
	}
	if _, err := database.Exec(`INSERT INTO users(id,email) VALUES('usr_group_migration','member@example.com')`); err != nil {
		t.Fatal(err)
	}
	if _, err := database.Exec(`
		INSERT INTO google_group_memberships(user_id,group_id,synced_at)
		VALUES('usr_group_migration','groups/one',now());
		INSERT INTO google_group_sync_state(user_id,last_attempt_at,last_success_at)
		VALUES('usr_group_migration',now(),now())`); err != nil {
		t.Fatal(err)
	}
	if _, err := database.Exec(`DELETE FROM google_groups WHERE id='groups/one'`); err != nil {
		t.Fatal(err)
	}
	var memberships int
	if err := database.QueryRow(`SELECT count(*) FROM google_group_memberships`).Scan(&memberships); err != nil || memberships != 0 {
		t.Fatalf("group deletion did not cascade memberships: count=%d err=%v", memberships, err)
	}
	if _, err := database.Exec(`DELETE FROM users WHERE id='usr_group_migration'`); err != nil {
		t.Fatal(err)
	}
	var syncStates int
	if err := database.QueryRow(`SELECT count(*) FROM google_group_sync_state`).Scan(&syncStates); err != nil || syncStates != 0 {
		t.Fatalf("user deletion did not cascade sync state: count=%d err=%v", syncStates, err)
	}
	if err := goose.DownTo(database, "migrations", 17); err != nil {
		t.Fatal(err)
	}
	var exists bool
	if err := database.QueryRow(`SELECT to_regclass('google_groups') IS NOT NULL`).Scan(&exists); err != nil || exists {
		t.Fatalf("google_groups remains after down migration: exists=%v err=%v", exists, err)
	}
	if err := goose.Up(database, "migrations"); err != nil {
		t.Fatalf("migration did not re-apply after down: %v", err)
	}
}
