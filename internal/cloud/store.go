package cloud

import (
	"context"
	"database/sql"
	"embed"
	"fmt"
	_ "github.com/jackc/pgx/v5/stdlib"
	"sort"
	"time"
)

//go:embed migrations/*.sql
var migrations embed.FS

type Store struct{ DB *sql.DB }

func OpenStore(ctx context.Context, dsn string) (*Store, error) {
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(20)
	db.SetMaxIdleConns(5)
	db.SetConnMaxLifetime(30 * time.Minute)
	if err = db.PingContext(ctx); err != nil {
		db.Close()
		return nil, fmt.Errorf("connect PostgreSQL: %w", err)
	}
	return &Store{DB: db}, nil
}
func (s *Store) Close() error { return s.DB.Close() }
func (s *Store) Migrate(ctx context.Context) error {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err = tx.ExecContext(ctx, "SELECT pg_advisory_xact_lock(814037219)"); err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, "CREATE TABLE IF NOT EXISTS cloud_schema_migrations (name text PRIMARY KEY, applied_at timestamptz NOT NULL DEFAULT now())"); err != nil {
		return err
	}
	entries, err := migrations.ReadDir("migrations")
	if err != nil {
		return err
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].Name() < entries[j].Name() })
	for _, entry := range entries {
		var exists bool
		if err = tx.QueryRowContext(ctx, "SELECT EXISTS(SELECT 1 FROM cloud_schema_migrations WHERE name=$1)", entry.Name()).Scan(&exists); err != nil {
			return err
		}
		if exists {
			continue
		}
		b, e := migrations.ReadFile("migrations/" + entry.Name())
		if e != nil {
			return e
		}
		if _, err = tx.ExecContext(ctx, string(b)); err != nil {
			return fmt.Errorf("migration %s: %w", entry.Name(), err)
		}
		if _, err = tx.ExecContext(ctx, "INSERT INTO cloud_schema_migrations(name) VALUES($1)", entry.Name()); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// CheckMigrations performs no DDL and is safe for the restricted runtime role.
// Both missing migrations and unknown future migrations reject startup.
func (s *Store) CheckMigrations(ctx context.Context) error {
	return checkMigrationNames(ctx, s.DB)
}

type migrationReader interface {
	QueryContext(context.Context, string, ...any) (*sql.Rows, error)
}

func checkMigrationNames(ctx context.Context, db migrationReader) error {
	entries, e := migrations.ReadDir("migrations")
	if e != nil {
		return e
	}
	expected := map[string]bool{}
	for _, entry := range entries {
		if !entry.IsDir() {
			expected[entry.Name()] = false
		}
	}
	rows, e := db.QueryContext(ctx, `SELECT name FROM cloud_schema_migrations`)
	if e != nil {
		return fmt.Errorf("schema not initialized; run cloud-migrate migrate with owner credentials: %w", e)
	}
	defer rows.Close()
	for rows.Next() {
		var name string
		if e = rows.Scan(&name); e != nil {
			return e
		}
		if _, ok := expected[name]; !ok {
			return fmt.Errorf("database migration %s is newer or unknown to this binary; deploy a compatible version", name)
		}
		expected[name] = true
	}
	if e = rows.Err(); e != nil {
		return e
	}
	missing := []string{}
	for name, applied := range expected {
		if !applied {
			missing = append(missing, name)
		}
	}
	sort.Strings(missing)
	if len(missing) > 0 {
		return fmt.Errorf("database requires migrations %v; run cloud-migrate migrate before deployment", missing)
	}
	return nil
}

// CheckRuntimeRole rejects privileged database credentials in long-lived API and
// worker services. Operator commands intentionally do not call this method.
func (s *Store) CheckRuntimeRole(ctx context.Context) error {
	var unsafe bool
	err := s.DB.QueryRowContext(ctx, `SELECT r.rolsuper OR r.rolcreatedb OR r.rolcreaterole OR r.rolreplication OR r.rolbypassrls OR EXISTS(SELECT 1 FROM pg_auth_members m WHERE m.member=r.oid) OR has_schema_privilege(current_user,'public','CREATE') OR has_database_privilege(current_user,current_database(),'CREATE') OR has_database_privilege(current_user,current_database(),'TEMP') OR has_table_privilege(current_user,'public.cloud_schema_migrations','INSERT,UPDATE,DELETE,TRUNCATE') OR has_table_privilege(current_user,'public.cloud_admin_roles','INSERT,UPDATE,DELETE,TRUNCATE') OR EXISTS(SELECT 1 FROM pg_class c JOIN pg_namespace n ON n.oid=c.relnamespace WHERE c.relowner=r.oid AND n.nspname='public') FROM pg_roles r WHERE r.rolname=current_user`).Scan(&unsafe)
	if err != nil {
		return fmt.Errorf("check runtime database privileges: %w", err)
	}
	if unsafe {
		return fmt.Errorf("production runtime requires a restricted, non-owner database role; provision with cloud-migrate provision-role")
	}
	return nil
}
