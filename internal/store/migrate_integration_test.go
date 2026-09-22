package store

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// testStore connects to the development database, or skips. Deliberately not a container of its own:
// the Compose Postgres is the one this app runs against (see the removed capture integration test for
// the type-inference bug that justified this choice).
func testStore(t *testing.T) *Store {
	t.Helper()
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		dsn = "postgres://astro:astro@localhost:5432/astrostack?sslmode=disable"
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	st, err := New(ctx, dsn)
	if err != nil {
		t.Skipf("no development database reachable (%v) — run `just up`", err)
	}
	t.Cleanup(st.Close)
	return st
}

// droppedTables are the tables whose features left the fork in E01 (S3 connections/ledger, capture
// sessions, tracking, capture conditions). mosaic_plans, equipment_setups and agent_series stay.
var droppedTables = []string{
	"s3_connections", "s3_objects",
	"capture_sequences", "capture_sessions", "capture_frames",
	"capture_conditions", "capture_forecasts",
	"tracking_samples",
}

func tableExists(t *testing.T, st *Store, name string) bool {
	t.Helper()
	var exists bool
	err := st.pool.QueryRow(context.Background(),
		`SELECT EXISTS (SELECT 1 FROM information_schema.tables WHERE table_schema='public' AND table_name=$1)`,
		name).Scan(&exists)
	require.NoError(t, err)
	return exists
}

// TestMigrate_PruneDroppedFeatures_UpDown proves migration 0023 round-trips on the real database:
// up drops every dead table, down recreates the schema, up drops it again.
func TestMigrate_PruneDroppedFeatures_UpDown(t *testing.T) {
	st := testStore(t)
	ctx := context.Background()

	_, err := st.Migrate(ctx)
	require.NoError(t, err)
	for _, tbl := range droppedTables {
		assert.False(t, tableExists(t, st, tbl), "%s must be dropped after migrating up", tbl)
	}
	// Kept neighbours prove the drop did not overshoot.
	for _, tbl := range []string{"mosaic_plans", "equipment_setups", "agent_series", "jobs"} {
		assert.True(t, tableExists(t, st, tbl), "%s must survive the prune", tbl)
	}

	require.NoError(t, st.MigrateDown(ctx))
	for _, tbl := range droppedTables {
		assert.True(t, tableExists(t, st, tbl), "%s must be recreated by the down migration", tbl)
	}

	applied, err := st.Migrate(ctx)
	require.NoError(t, err)
	assert.Equal(t, 1, applied, "re-applying should run exactly the prune migration")
	for _, tbl := range droppedTables {
		assert.False(t, tableExists(t, st, tbl), "%s must be dropped again on re-up", tbl)
	}
}
