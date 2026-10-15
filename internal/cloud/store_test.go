package cloud

import (
	"context"
	"testing"
)

func TestCheckMigrationsIsReadOnlyAndRejectsFutureSchema(t *testing.T) {
	s, _ := testPlatform(t)
	ctx := context.Background()
	if e := s.CheckMigrations(ctx); e != nil {
		t.Fatal(e)
	}
	tx, e := s.DB.BeginTx(ctx, nil)
	if e != nil {
		t.Fatal(e)
	}
	defer tx.Rollback()
	// A future schema ledger entry cannot be interpreted safely by old binaries.
	_, e = tx.ExecContext(ctx, `INSERT INTO cloud_schema_migrations(name)VALUES('999_future_probe.sql')`)
	if e != nil {
		t.Fatal(e)
	}
	// Run check against the transaction so this probe never modifies shared state.
	if e = checkMigrationNames(ctx, tx); e == nil {
		t.Fatal("future schema accepted")
	}
}
