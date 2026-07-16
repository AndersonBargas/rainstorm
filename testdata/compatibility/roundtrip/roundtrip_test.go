package roundtrip

import (
	"context"
	"testing"

	rainstormv5 "github.com/AndersonBargas/rainstorm/v5"
	rainstormv6 "github.com/AndersonBargas/rainstorm/v6"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// RoundtripRecord is the shared schema for the roundtrip test.
// It uses the v5.3.0 tag syntax which is compatible with both versions.
type RoundtripRecord struct {
	ID       uint64 `rainstorm:"id,increment"`
	Key      string `rainstorm:"unique"`
	Category string `rainstorm:"index"`
	Name     string
	Revision int
}

// =========================================================================
// Bidirectional roundtrip: v5 → v6 → v5 → v6
// =========================================================================

func TestRoundtrip_Bidirectional(t *testing.T) {
	dir := t.TempDir()
	dbPath := dir + "/roundtrip.db"
	ctx := context.Background()

	// ------------------------------------------------------------------
	// Phase 1: v5 creates the database with default JSON codec.
	// ------------------------------------------------------------------
	t.Run("Phase1_v5Create", func(t *testing.T) {
		db, err := rainstormv5.Open(dbPath)
		require.NoError(t, err, "v5 open")

		// Three root records: IDs 1, 2, 3.
		records := []RoundtripRecord{
			{Key: "alpha", Category: "shared", Name: "Alpha", Revision: 1},
			{Key: "beta", Category: "shared", Name: "Beta", Revision: 2},
			{Key: "gamma", Category: "other", Name: "Gamma", Revision: 3},
		}
		for i := range records {
			err := db.Save(&records[i])
			require.NoError(t, err, "v5 save record %d", i)
			require.Equal(t, uint64(i+1), records[i].ID, "v5 record %d ID", i)
		}

		// Root KV.
		err = db.Set("settings", "name", "roundtrip-fixture")
		require.NoError(t, err, "v5 kv set name")

		// Nested record under "tenant/acme".
		nestedNode := db.From("tenant", "acme")
		nested := RoundtripRecord{
			Key:      "nested-alpha",
			Category: "nested",
			Name:     "Nested Alpha",
			Revision: 10,
		}
		err = nestedNode.Save(&nested)
		require.NoError(t, err, "v5 nested save")
		require.Equal(t, uint64(1), nested.ID, "v5 nested ID")

		err = db.Close()
		require.NoError(t, err, "v5 close")
	})

	// ------------------------------------------------------------------
	// Phase 2: v6 reads and mutates.
	// ------------------------------------------------------------------
	t.Run("Phase2_v6Mutate", func(t *testing.T) {
		db, err := rainstormv6.Open(ctx, dbPath)
		require.NoError(t, err, "v6 open")
		defer func() { require.NoError(t, db.Close(), "v6 close") }()

		var all []RoundtripRecord
		err = db.All(ctx, &all)
		require.NoError(t, err, "v6 All")
		require.Len(t, all, 3)

		byKey := make(map[string]RoundtripRecord, 3)
		for _, r := range all {
			byKey[r.Key] = r
		}
		require.Contains(t, byKey, "alpha")
		assert.Equal(t, uint64(1), byKey["alpha"].ID)
		require.Contains(t, byKey, "beta")
		assert.Equal(t, uint64(2), byKey["beta"].ID)
		require.Contains(t, byKey, "gamma")
		assert.Equal(t, uint64(3), byKey["gamma"].ID)

		// Verify Category index.
		var shared []RoundtripRecord
		err = db.Find(ctx, "Category", "shared", &shared)
		require.NoError(t, err)
		require.Len(t, shared, 2)
		assert.Equal(t, "alpha", shared[0].Key)
		assert.Equal(t, "beta", shared[1].Key)

		var other []RoundtripRecord
		err = db.Find(ctx, "Category", "other", &other)
		require.NoError(t, err)
		require.Len(t, other, 1)
		assert.Equal(t, "gamma", other[0].Key)

		// Verify unique Key lookups.
		var alpha RoundtripRecord
		err = db.One(ctx, "Key", "alpha", &alpha)
		require.NoError(t, err)
		assert.Equal(t, uint64(1), alpha.ID)

		// Read KV.
		var kvName string
		err = db.Get(ctx, "settings", "name", &kvName)
		require.NoError(t, err)
		assert.Equal(t, "roundtrip-fixture", kvName)

		// Read nested record.
		nestedNode := db.From("tenant", "acme")
		var nestedAll []RoundtripRecord
		err = nestedNode.All(ctx, &nestedAll)
		require.NoError(t, err)
		require.Len(t, nestedAll, 1)
		assert.Equal(t, "nested-alpha", nestedAll[0].Key)

		// --- Mutations ---

		// 1. Update alpha: Category shared → migrated.
		alpha.Category = "migrated"
		alpha.Name = "Alpha V6 Updated"
		alpha.Revision = 100
		err = db.Update(ctx, &alpha)
		require.NoError(t, err, "v6 update alpha")

		// shared should now contain only beta.
		var sharedAfter []RoundtripRecord
		err = db.Find(ctx, "Category", "shared", &sharedAfter)
		require.NoError(t, err)
		require.Len(t, sharedAfter, 1)
		assert.Equal(t, "beta", sharedAfter[0].Key)

		// migrated contains alpha.
		var migrated []RoundtripRecord
		err = db.Find(ctx, "Category", "migrated", &migrated)
		require.NoError(t, err)
		require.Len(t, migrated, 1)
		assert.Equal(t, "alpha", migrated[0].Key)
		assert.Equal(t, "Alpha V6 Updated", migrated[0].Name)

		// 2. Delete beta.
		var beta RoundtripRecord
		err = db.One(ctx, "Key", "beta", &beta)
		require.NoError(t, err)
		err = db.DeleteStruct(ctx, &beta)
		require.NoError(t, err, "v6 delete beta")

		err = db.One(ctx, "Key", "beta", &RoundtripRecord{})
		require.ErrorIs(t, err, rainstormv6.ErrNotFound, "v6: beta deleted")

		var sharedEmpty []RoundtripRecord
		err = db.Find(ctx, "Category", "shared", &sharedEmpty)
		require.ErrorIs(t, err, rainstormv6.ErrNotFound, "v6: shared should be empty")

		// 3. Reuse Key "beta" in a replacement (ID = 4).
		replacement := RoundtripRecord{
			Key:      "beta",
			Category: "replacement",
			Name:     "Replacement Beta",
			Revision: 99,
		}
		err = db.Save(ctx, &replacement)
		require.NoError(t, err, "v6 save replacement")
		assert.Equal(t, uint64(4), replacement.ID, "v6 replacement ID")

		// 4. Save a new record (ID = 5).
		newRec := RoundtripRecord{
			Key:      "delta",
			Category: "new",
			Name:     "Delta V6",
			Revision: 50,
		}
		err = db.Save(ctx, &newRec)
		require.NoError(t, err, "v6 save delta")
		assert.Equal(t, uint64(5), newRec.ID, "v6 delta ID")

		// 5. Update KV.
		err = db.Set(ctx, "settings", "name", "v6-updated-fixture")
		require.NoError(t, err, "v6 kv update")

		// 6. Update nested record.
		nestedAll[0].Name = "Nested V6 Updated"
		nestedAll[0].Revision = 200
		err = nestedNode.Update(ctx, &nestedAll[0])
		require.NoError(t, err, "v6 nested update")
	})

	// ------------------------------------------------------------------
	// Phase 3: v5 reopens and verifies.
	// ------------------------------------------------------------------
	t.Run("Phase3_v5Reopen", func(t *testing.T) {
		db, err := rainstormv5.Open(dbPath)
		require.NoError(t, err, "v5 reopen")
		defer func() { require.NoError(t, db.Close(), "v5 close after reopen") }()

		// Complete state before epsilon: exactly IDs 1, 3, 4, 5.
		var all []RoundtripRecord
		err = db.All(&all)
		require.NoError(t, err, "v5: All before epsilon")
		require.Len(t, all, 4)

		byKey := make(map[string]RoundtripRecord, 4)
		byID := make(map[uint64]RoundtripRecord, 4)
		for _, r := range all {
			byKey[r.Key] = r
			byID[r.ID] = r
		}

		// ID 2 is absent from the complete result.
		assert.NotContains(t, byID, uint64(2), "v5: old beta ID 2 absent from All")

		// Exact IDs and Keys.
		require.Contains(t, byID, uint64(1))
		require.Contains(t, byKey, "alpha")
		assert.Equal(t, uint64(1), byID[1].ID)
		assert.Equal(t, "migrated", byID[1].Category, "v5: alpha in migrated")
		assert.Equal(t, "Alpha V6 Updated", byID[1].Name)

		require.Contains(t, byID, uint64(3))
		assert.Equal(t, "gamma", byID[3].Key)
		assert.Equal(t, "other", byID[3].Category)

		require.Contains(t, byID, uint64(4))
		assert.Equal(t, "beta", byID[4].Key, "v5: replacement at ID 4 has Key beta")
		assert.Equal(t, "replacement", byID[4].Category)
		assert.Equal(t, "Replacement Beta", byID[4].Name)

		require.Contains(t, byID, uint64(5))
		assert.Equal(t, "delta", byID[5].Key)
		assert.Equal(t, "new", byID[5].Category)

		// Read alpha via unique Key.
		var alpha RoundtripRecord
		err = db.One("Key", "alpha", &alpha)
		require.NoError(t, err, "v5 read alpha")
		assert.Equal(t, uint64(1), alpha.ID)
		assert.Equal(t, "migrated", alpha.Category)
		assert.Equal(t, "Alpha V6 Updated", alpha.Name)
		assert.Equal(t, 100, alpha.Revision)

		// Old index "shared" is empty.
		var shared []RoundtripRecord
		err = db.Find("Category", "shared", &shared)
		require.ErrorIs(t, err, rainstormv5.ErrNotFound, "v5: shared should be empty")
		require.Empty(t, shared)

		// "migrated" contains alpha.
		var migrated []RoundtripRecord
		err = db.Find("Category", "migrated", &migrated)
		require.NoError(t, err, "v5: find migrated")
		require.Len(t, migrated, 1)
		assert.Equal(t, "alpha", migrated[0].Key)

		// "replacement" index contains ID 4.
		var replIdx []RoundtripRecord
		err = db.Find("Category", "replacement", &replIdx)
		require.NoError(t, err, "v5: replacement index")
		require.Len(t, replIdx, 1)
		assert.Equal(t, uint64(4), replIdx[0].ID)

		// "new" index contains delta (ID 5).
		var newIdx []RoundtripRecord
		err = db.Find("Category", "new", &newIdx)
		require.NoError(t, err, "v5: new index")
		require.Len(t, newIdx, 1)
		assert.Equal(t, uint64(5), newIdx[0].ID)
		assert.Equal(t, "delta", newIdx[0].Key)

		// Old beta (ID 2) deleted — lookup by ID must fail.
		err = db.One("ID", uint64(2), &RoundtripRecord{})
		require.ErrorIs(t, err, rainstormv5.ErrNotFound, "v5: old beta ID 2 deleted")

		// Gamma unchanged.
		var gamma RoundtripRecord
		err = db.One("Key", "gamma", &gamma)
		require.NoError(t, err, "v5: gamma unchanged")
		assert.Equal(t, uint64(3), gamma.ID)
		assert.Equal(t, "other", gamma.Category)

		// KV update readable.
		var kvName string
		err = db.Get("settings", "name", &kvName)
		require.NoError(t, err, "v5: kv name")
		assert.Equal(t, "v6-updated-fixture", kvName)

		// Nested update readable.
		nestedNode := db.From("tenant", "acme")
		var nestedAll []RoundtripRecord
		err = nestedNode.All(&nestedAll)
		require.NoError(t, err, "v5: nested all")
		require.Len(t, nestedAll, 1)
		assert.Equal(t, "Nested V6 Updated", nestedAll[0].Name)
		assert.Equal(t, 200, nestedAll[0].Revision)

		// --- One additional v5 write ---
		extra := RoundtripRecord{
			Key:      "epsilon",
			Category: "extra",
			Name:     "Epsilon V5",
			Revision: 300,
		}
		err = db.Save(&extra)
		require.NoError(t, err, "v5 save epsilon")
		assert.Equal(t, uint64(6), extra.ID, "v5 epsilon ID = 6")
	})

	// ------------------------------------------------------------------
	// Phase 4: v6 final reopen, verify v5's final write.
	// ------------------------------------------------------------------
	t.Run("Phase4_v6FinalReopen", func(t *testing.T) {
		db, err := rainstormv6.Open(ctx, dbPath)
		require.NoError(t, err, "v6 final reopen")
		defer func() { require.NoError(t, db.Close(), "v6 final close") }()

		// All must equal exact IDs 1, 3, 4, 5, 6 and exact Keys.
		var all []RoundtripRecord
		err = db.All(ctx, &all)
		require.NoError(t, err, "v6: All final")
		require.Len(t, all, 5)

		byKey := make(map[string]RoundtripRecord, 5)
		byID := make(map[uint64]RoundtripRecord, 5)
		for _, r := range all {
			byKey[r.Key] = r
			byID[r.ID] = r
		}

		// Exact records.
		require.Contains(t, byID, uint64(1))
		assert.Equal(t, "alpha", byID[1].Key)
		assert.Equal(t, "migrated", byID[1].Category, "v6: alpha in migrated")

		require.Contains(t, byID, uint64(3))
		assert.Equal(t, "gamma", byID[3].Key)
		assert.Equal(t, "other", byID[3].Category)

		require.Contains(t, byID, uint64(4))
		assert.Equal(t, "beta", byID[4].Key, "v6: replacement at ID 4")
		assert.Equal(t, "replacement", byID[4].Category)

		require.Contains(t, byID, uint64(5))
		assert.Equal(t, "delta", byID[5].Key)
		assert.Equal(t, "new", byID[5].Category)

		require.Contains(t, byID, uint64(6))
		assert.Equal(t, "epsilon", byID[6].Key)
		assert.Equal(t, "extra", byID[6].Category)
		assert.Equal(t, "Epsilon V5", byID[6].Name)

		// Old beta (ID 2) is absent.
		assert.NotContains(t, byID, uint64(2), "v6: old beta ID 2 still absent")

		// Epsilon indexed correctly.
		var extra []RoundtripRecord
		err = db.Find(ctx, "Category", "extra", &extra)
		require.NoError(t, err, "v6: find extra")
		require.Len(t, extra, 1)
		assert.Equal(t, "epsilon", extra[0].Key)

		// KV still updated.
		var kvName string
		err = db.Get(ctx, "settings", "name", &kvName)
		require.NoError(t, err)
		assert.Equal(t, "v6-updated-fixture", kvName)

		// Nested still updated.
		nestedNode := db.From("tenant", "acme")
		var nestedAll []RoundtripRecord
		err = nestedNode.All(ctx, &nestedAll)
		require.NoError(t, err)
		require.Len(t, nestedAll, 1)
		assert.Equal(t, "Nested V6 Updated", nestedAll[0].Name)
	})
}
