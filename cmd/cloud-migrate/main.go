// cloud-migrate is an explicit operator command. Runtime containers never need
// owner credentials, CREATE privileges, or access to the master RDS secret.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"regexp"
	"strings"
	"time"

	"videoflow-go/internal/cloud"
)

func main() {
	if e := run(); e != nil {
		fmt.Fprintln(os.Stderr, e)
		os.Exit(1)
	}
}
func run() error {
	if len(os.Args) < 2 {
		return errors.New("usage: cloud-migrate migrate|check|provision-role [--role videoflow_runtime]")
	}
	command := os.Args[1]
	flags := flag.NewFlagSet(command, flag.ContinueOnError)
	role := flags.String("role", "videoflow_runtime", "restricted application login role")
	if e := flags.Parse(os.Args[2:]); e != nil {
		return e
	}
	if flags.NArg() != 0 {
		return errors.New("unexpected positional arguments")
	}
	dsn := os.Getenv("MIGRATION_DATABASE_URL")
	if command == "check" {
		dsn = os.Getenv("DATABASE_URL")
	}
	if dsn == "" {
		return errors.New("migrate/provision-role require MIGRATION_DATABASE_URL; check requires DATABASE_URL")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	store, e := cloud.OpenStore(ctx, dsn)
	if e != nil {
		return e
	}
	defer store.Close()
	switch command {
	case "migrate":
		e = store.Migrate(ctx)
	case "check":
		e = store.CheckMigrations(ctx)
	case "provision-role":
		if e = store.CheckMigrations(ctx); e != nil {
			return e
		}
		password := os.Getenv("RUNTIME_DATABASE_PASSWORD")
		if len(password) < 20 || len(password) > 1024 {
			return errors.New("RUNTIME_DATABASE_PASSWORD must contain 20..1024 bytes")
		}
		e = provisionRuntimeRole(ctx, store, *role, password)
	default:
		return errors.New("unknown command; expected migrate, check, or provision-role")
	}
	if e != nil {
		return e
	}
	fmt.Println(command + " completed")
	return nil
}

var rolePattern = regexp.MustCompile(`^[a-z][a-z0-9_]{2,62}$`)

func validRoleName(role string) bool {
	return rolePattern.MatchString(role) && role != "postgres" && !strings.HasPrefix(role, "pg_") && !strings.HasPrefix(role, "rds")
}
func quoteIdentifier(value string) string { return `"` + strings.ReplaceAll(value, `"`, `""`) + `"` }
func quoteLiteral(value string) string    { return "'" + strings.ReplaceAll(value, "'", "''") + "'" }
func provisionRuntimeRole(ctx context.Context, store *cloud.Store, role, password string) error {
	if !validRoleName(role) || len(password) < 20 || strings.ContainsRune(password, 0) {
		return errors.New("invalid runtime role or password")
	}
	tx, e := store.DB.BeginTx(ctx, nil)
	if e != nil {
		return e
	}
	defer tx.Rollback()
	if _, e = tx.ExecContext(ctx, `SELECT pg_advisory_xact_lock(814037224)`); e != nil {
		return e
	}
	if _, e = tx.ExecContext(ctx, `SET LOCAL standard_conforming_strings=on`); e != nil {
		return e
	}
	var current, database string
	if e = tx.QueryRowContext(ctx, `SELECT current_user,current_database()`).Scan(&current, &database); e != nil {
		return e
	}
	if current == role {
		return errors.New("provisioning identity must differ from runtime identity")
	}
	var exists bool
	if e = tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM pg_roles WHERE rolname=$1)`, role).Scan(&exists); e != nil {
		return e
	}
	if exists {
		var unsafe bool
		e = tx.QueryRowContext(ctx, `SELECT r.rolsuper OR r.rolcreatedb OR r.rolcreaterole OR r.rolreplication OR r.rolbypassrls OR EXISTS(SELECT 1 FROM pg_auth_members m WHERE m.member=r.oid) OR EXISTS(SELECT 1 FROM pg_class c WHERE c.relowner=r.oid) OR EXISTS(SELECT 1 FROM pg_namespace n WHERE n.nspowner=r.oid) OR EXISTS(SELECT 1 FROM pg_database d WHERE d.datdba=r.oid) FROM pg_roles r WHERE r.rolname=$1`, role).Scan(&unsafe)
		if e != nil {
			return e
		}
		if unsafe {
			return errors.New("existing runtime role owns objects, has elevated privileges, or has role memberships; use a new restricted role")
		}
	} else {
		if _, e = tx.ExecContext(ctx, `CREATE ROLE `+quoteIdentifier(role)+` LOGIN NOSUPERUSER NOCREATEDB NOCREATEROLE NOREPLICATION NOBYPASSRLS NOINHERIT`); e != nil {
			return e
		}
	}
	statements := []string{
		`ALTER ROLE ` + quoteIdentifier(role) + ` LOGIN NOSUPERUSER NOCREATEDB NOCREATEROLE NOREPLICATION NOBYPASSRLS NOINHERIT PASSWORD ` + quoteLiteral(password),
		`ALTER ROLE ` + quoteIdentifier(role) + ` SET search_path=pg_catalog,public`,
		`ALTER ROLE ` + quoteIdentifier(role) + ` SET statement_timeout='30s'`,
		`ALTER ROLE ` + quoteIdentifier(role) + ` SET lock_timeout='5s'`,
		`ALTER ROLE ` + quoteIdentifier(role) + ` SET idle_in_transaction_session_timeout='60s'`,
		`REVOKE CREATE ON SCHEMA public FROM PUBLIC`,
		`REVOKE TEMPORARY ON DATABASE ` + quoteIdentifier(database) + ` FROM PUBLIC`,
		`REVOKE ALL ON DATABASE ` + quoteIdentifier(database) + ` FROM ` + quoteIdentifier(role),
		`GRANT CONNECT ON DATABASE ` + quoteIdentifier(database) + ` TO ` + quoteIdentifier(role),
		`REVOKE ALL ON SCHEMA public FROM ` + quoteIdentifier(role),
		`GRANT USAGE ON SCHEMA public TO ` + quoteIdentifier(role),
		`REVOKE ALL ON ALL TABLES IN SCHEMA public FROM ` + quoteIdentifier(role),
		`REVOKE ALL ON ALL SEQUENCES IN SCHEMA public FROM ` + quoteIdentifier(role),
		`GRANT SELECT ON public.cloud_schema_migrations TO ` + quoteIdentifier(role),
	}
	for _, statement := range statements {
		if _, e = tx.ExecContext(ctx, statement); e != nil {
			return fmt.Errorf("provision runtime privileges: %w", e)
		}
	}
	// Exact existing application tables only. New migrations require a reviewed
	// re-grant; future operator tables never acquire runtime access by default.
	rows, e := tx.QueryContext(ctx, `SELECT tablename FROM pg_tables WHERE schemaname='public' AND left(tablename,6)='cloud_' AND tablename NOT IN ('cloud_schema_migrations','cloud_admin_audit','cloud_payout_batches','cloud_payout_batch_entries','cloud_reward_periods') ORDER BY tablename`)
	if e != nil {
		return e
	}
	tables := []string{}
	for rows.Next() {
		var name string
		if e = rows.Scan(&name); e != nil {
			rows.Close()
			return e
		}
		tables = append(tables, name)
	}
	e = rows.Err()
	rows.Close()
	if e != nil {
		return e
	}
	for _, table := range tables {
		privileges := "SELECT,INSERT,UPDATE,DELETE"
		switch table {
		case "cloud_credit_entries", "cloud_credit_postings", "cloud_money_postings", "cloud_earnings", "cloud_premium_refunds", "cloud_premium_invoices":
			privileges = "SELECT,INSERT"
		}
		if _, e = tx.ExecContext(ctx, `GRANT `+privileges+` ON public.`+quoteIdentifier(table)+` TO `+quoteIdentifier(role)); e != nil {
			return e
		}
	}
	if _, e = tx.ExecContext(ctx, `GRANT UPDATE(refunded_cents) ON public.cloud_premium_invoices TO `+quoteIdentifier(role)); e != nil {
		return e
	}
	// Webhook dispute audits are append-only. Financial operators use a separate login.
	if _, e = tx.ExecContext(ctx, `GRANT INSERT ON public.cloud_admin_audit TO `+quoteIdentifier(role)); e != nil {
		return e
	}
	// Serial IDs used by triggers need USAGE, never sequence ownership/DDL.
	if _, e = tx.ExecContext(ctx, `GRANT USAGE,SELECT ON ALL SEQUENCES IN SCHEMA public TO `+quoteIdentifier(role)); e != nil {
		return e
	}
	return tx.Commit()
}
