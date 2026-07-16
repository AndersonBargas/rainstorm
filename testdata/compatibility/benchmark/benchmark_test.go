// Package benchmark provides cross-major benchmarks comparing rainstorm v5.3.0
// and the current local v6 implementation.
//
// Both major versions compile into the same benchmark binary. This proves
// relative API implementation behaviour under one selected dependency graph.
// It is not process / toolchain isolation.
package benchmark

import (
	"context"
	"errors"
	"fmt"
	"testing"

	rainstormv5 "github.com/AndersonBargas/rainstorm/v5"
	rainstormv6 "github.com/AndersonBargas/rainstorm/v6"
	bolt "go.etcd.io/bbolt"
)

// ---------------------------------------------------------------------------
// Shared types and constants
// ---------------------------------------------------------------------------

// BenchmarkRecord is the shared schema for all benchmarks.
// It uses the rainstorm tag syntax compatible with both v5 and v6.
type BenchmarkRecord struct {
	ID       uint64 `rainstorm:"id,increment"`
	Key      string `rainstorm:"unique"`
	Category string `rainstorm:"index"`
	Name     string
	Revision int
	Payload  string
}

// payload100 is a deterministic 100-byte payload to make reads and writes
// non-trivial without dominating the operation cost.
const payload100 = "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef0123"

// benchBoltOptsV5 disables fsync for benchmark databases created in temp dirs.
// This accelerates population and defines the write configuration measured by
// this benchmark suite. Results do not characterize fsync-enabled writes.
var benchBoltOptsV5 = rainstormv5.BoltOptions(0600, &bolt.Options{NoSync: true})

// benchBoltOptsV6 is the v6-equivalent NoSync option.
var benchBoltOptsV6 = rainstormv6.BoltOptions(0600, &bolt.Options{NoSync: true})

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// populateRecordsV5 saves n deterministic BenchmarkRecord instances into the
// v5 database and returns the resulting slice (with IDs assigned).
func populateRecordsV5(b *testing.B, db *rainstormv5.DB, n int) []BenchmarkRecord {
	b.Helper()
	records := make([]BenchmarkRecord, n)
	half := n / 2
	for i := 0; i < n; i++ {
		cat := "cat-a"
		nameCat := "name-a"
		if i >= half {
			cat = "cat-b"
			nameCat = "name-b"
		}
		rec := BenchmarkRecord{
			Key:      fmt.Sprintf("key-%04d", i),
			Category: cat,
			Name:     nameCat,
			Revision: i,
			Payload:  payload100,
		}
		if err := db.Save(&rec); err != nil {
			b.Fatalf("populate v5 save %d: %v", i, err)
		}
		records[i] = rec
	}
	return records
}

// populateRecordsV6 saves n deterministic BenchmarkRecord instances into the
// v6 database and returns the resulting slice (with IDs assigned).
func populateRecordsV6(b *testing.B, db *rainstormv6.DB, n int) []BenchmarkRecord {
	b.Helper()
	ctx := context.Background()
	records := make([]BenchmarkRecord, n)
	half := n / 2
	for i := 0; i < n; i++ {
		cat := "cat-a"
		nameCat := "name-a"
		if i >= half {
			cat = "cat-b"
			nameCat = "name-b"
		}
		rec := BenchmarkRecord{
			Key:      fmt.Sprintf("key-%04d", i),
			Category: cat,
			Name:     nameCat,
			Revision: i,
			Payload:  payload100,
		}
		if err := db.Save(ctx, &rec); err != nil {
			b.Fatalf("populate v6 save %d: %v", i, err)
		}
		records[i] = rec
	}
	return records
}

// preflightReadV5 performs a correctness check on a populated v5 database.
func preflightReadV5(b *testing.B, db *rainstormv5.DB, n int) {
	b.Helper()

	// All records must be present.
	var all []BenchmarkRecord
	if err := db.All(&all); err != nil {
		b.Fatalf("preflight v5 All: %v", err)
	}
	if len(all) != n {
		b.Fatalf("preflight v5 All: want %d, got %d", n, len(all))
	}

	// OneByID: middle record must resolve.
	id := uint64(n / 2)
	if id == 0 {
		id = 1
	}
	var one BenchmarkRecord
	if err := db.One("ID", id, &one); err != nil {
		b.Fatalf("preflight v5 One ID=%d: %v", id, err)
	}
	if one.ID != id {
		b.Fatalf("preflight v5 One ID=%d: got ID=%d", id, one.ID)
	}

	// OneByUnique: a known key must resolve.
	knownKey := fmt.Sprintf("key-%04d", 0)
	var uniq BenchmarkRecord
	if err := db.One("Key", knownKey, &uniq); err != nil {
		b.Fatalf("preflight v5 One Key=%s: %v", knownKey, err)
	}
	if uniq.Key != knownKey {
		b.Fatalf("preflight v5 One Key: want %s, got %s", knownKey, uniq.Key)
	}

	// FindIndexed: Category=cat-a returns n/2 records.
	var indexed []BenchmarkRecord
	if err := db.Find("Category", "cat-a", &indexed); err != nil {
		b.Fatalf("preflight v5 Find Category=cat-a: %v", err)
	}
	expectedCatA := n / 2
	if n%2 != 0 {
		expectedCatA = n - n/2
	}
	if len(indexed) != expectedCatA {
		b.Fatalf("preflight v5 Find Category=cat-a: want %d, got %d", expectedCatA, len(indexed))
	}

	// FindUnindexed: Name=name-a must return the same count.
	var unindexed []BenchmarkRecord
	if err := db.Find("Name", "name-a", &unindexed); err != nil {
		b.Fatalf("preflight v5 Find Name=name-a: %v", err)
	}
	if len(unindexed) != expectedCatA {
		b.Fatalf("preflight v5 Find Name=name-a: want %d, got %d", expectedCatA, len(unindexed))
	}

	// KV baseline.
	if err := db.Set("bench_kv", "val", "hello"); err != nil {
		b.Fatalf("preflight v5 KV Set: %v", err)
	}
	var kvVal string
	if err := db.Get("bench_kv", "val", &kvVal); err != nil {
		b.Fatalf("preflight v5 KV Get: %v", err)
	}
	if kvVal != "hello" {
		b.Fatalf("preflight v5 KV: want hello, got %s", kvVal)
	}
}

// preflightReadV6 performs a correctness check on a populated v6 database.
func preflightReadV6(b *testing.B, db *rainstormv6.DB, n int) {
	b.Helper()
	ctx := context.Background()

	var all []BenchmarkRecord
	if err := db.All(ctx, &all); err != nil {
		b.Fatalf("preflight v6 All: %v", err)
	}
	if len(all) != n {
		b.Fatalf("preflight v6 All: want %d, got %d", n, len(all))
	}

	id := uint64(n / 2)
	if id == 0 {
		id = 1
	}
	var one BenchmarkRecord
	if err := db.One(ctx, "ID", id, &one); err != nil {
		b.Fatalf("preflight v6 One ID=%d: %v", id, err)
	}
	if one.ID != id {
		b.Fatalf("preflight v6 One ID=%d: got ID=%d", id, one.ID)
	}

	knownKey := fmt.Sprintf("key-%04d", 0)
	var uniq BenchmarkRecord
	if err := db.One(ctx, "Key", knownKey, &uniq); err != nil {
		b.Fatalf("preflight v6 One Key=%s: %v", knownKey, err)
	}
	if uniq.Key != knownKey {
		b.Fatalf("preflight v6 One Key: want %s, got %s", knownKey, uniq.Key)
	}

	var indexed []BenchmarkRecord
	if err := db.Find(ctx, "Category", "cat-a", &indexed); err != nil {
		b.Fatalf("preflight v6 Find Category=cat-a: %v", err)
	}
	expectedCatA := n / 2
	if n%2 != 0 {
		expectedCatA = n - n/2
	}
	if len(indexed) != expectedCatA {
		b.Fatalf("preflight v6 Find Category=cat-a: want %d, got %d", expectedCatA, len(indexed))
	}

	var unindexed []BenchmarkRecord
	if err := db.Find(ctx, "Name", "name-a", &unindexed); err != nil {
		b.Fatalf("preflight v6 Find Name=name-a: %v", err)
	}
	if len(unindexed) != expectedCatA {
		b.Fatalf("preflight v6 Find Name=name-a: want %d, got %d", expectedCatA, len(unindexed))
	}

	if err := db.Set(ctx, "bench_kv", "val", "hello"); err != nil {
		b.Fatalf("preflight v6 KV Set: %v", err)
	}
	var kvVal string
	if err := db.Get(ctx, "bench_kv", "val", &kvVal); err != nil {
		b.Fatalf("preflight v6 KV Get: %v", err)
	}
	if kvVal != "hello" {
		b.Fatalf("preflight v6 KV: want hello, got %s", kvVal)
	}
}

// =========================================================================
// V5 Benchmarks
// =========================================================================

func BenchmarkV5(b *testing.B) {
	// ---- OneByID ----
	b.Run("OneByID/100", func(b *testing.B) {
		dir := b.TempDir()
		db, err := rainstormv5.Open(dir+"/bench.db", benchBoltOptsV5)
		if err != nil {
			b.Fatal(err)
		}
		b.Cleanup(func() {
			if cerr := db.Close(); cerr != nil {
				b.Errorf("close: %v", cerr)
			}
		})

		n := 100
		_ = populateRecordsV5(b, db, n)
		preflightReadV5(b, db, n)

		lookupID := uint64(n / 2)

		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			var dst BenchmarkRecord
			if err := db.One("ID", lookupID, &dst); err != nil {
				b.Fatalf("One ID: %v", err)
			}
		}
	})

	b.Run("OneByID/1000", func(b *testing.B) {
		dir := b.TempDir()
		db, err := rainstormv5.Open(dir+"/bench.db", benchBoltOptsV5)
		if err != nil {
			b.Fatal(err)
		}
		b.Cleanup(func() {
			if cerr := db.Close(); cerr != nil {
				b.Errorf("close: %v", cerr)
			}
		})

		n := 1000
		_ = populateRecordsV5(b, db, n)
		preflightReadV5(b, db, n)

		lookupID := uint64(n / 2)

		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			var dst BenchmarkRecord
			if err := db.One("ID", lookupID, &dst); err != nil {
				b.Fatalf("One ID: %v", err)
			}
		}
	})

	// ---- OneByUnique ----
	b.Run("OneByUnique/100", func(b *testing.B) {
		dir := b.TempDir()
		db, err := rainstormv5.Open(dir+"/bench.db", benchBoltOptsV5)
		if err != nil {
			b.Fatal(err)
		}
		b.Cleanup(func() {
			if cerr := db.Close(); cerr != nil {
				b.Errorf("close: %v", cerr)
			}
		})

		n := 100
		_ = populateRecordsV5(b, db, n)
		preflightReadV5(b, db, n)

		lookupKey := fmt.Sprintf("key-%04d", n/2)

		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			var dst BenchmarkRecord
			if err := db.One("Key", lookupKey, &dst); err != nil {
				b.Fatalf("One Key: %v", err)
			}
		}
	})

	b.Run("OneByUnique/1000", func(b *testing.B) {
		dir := b.TempDir()
		db, err := rainstormv5.Open(dir+"/bench.db", benchBoltOptsV5)
		if err != nil {
			b.Fatal(err)
		}
		b.Cleanup(func() {
			if cerr := db.Close(); cerr != nil {
				b.Errorf("close: %v", cerr)
			}
		})

		n := 1000
		_ = populateRecordsV5(b, db, n)
		preflightReadV5(b, db, n)

		lookupKey := fmt.Sprintf("key-%04d", n/2)

		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			var dst BenchmarkRecord
			if err := db.One("Key", lookupKey, &dst); err != nil {
				b.Fatalf("One Key: %v", err)
			}
		}
	})

	// ---- FindIndexed ----
	b.Run("FindIndexed/100", func(b *testing.B) {
		dir := b.TempDir()
		db, err := rainstormv5.Open(dir+"/bench.db", benchBoltOptsV5)
		if err != nil {
			b.Fatal(err)
		}
		b.Cleanup(func() {
			if cerr := db.Close(); cerr != nil {
				b.Errorf("close: %v", cerr)
			}
		})

		n := 100
		_ = populateRecordsV5(b, db, n)
		preflightReadV5(b, db, n)

		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			var dst []BenchmarkRecord
			if err := db.Find("Category", "cat-a", &dst); err != nil {
				b.Fatalf("Find Category: %v", err)
			}
		}
	})

	b.Run("FindIndexed/1000", func(b *testing.B) {
		dir := b.TempDir()
		db, err := rainstormv5.Open(dir+"/bench.db", benchBoltOptsV5)
		if err != nil {
			b.Fatal(err)
		}
		b.Cleanup(func() {
			if cerr := db.Close(); cerr != nil {
				b.Errorf("close: %v", cerr)
			}
		})

		n := 1000
		_ = populateRecordsV5(b, db, n)
		preflightReadV5(b, db, n)

		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			var dst []BenchmarkRecord
			if err := db.Find("Category", "cat-a", &dst); err != nil {
				b.Fatalf("Find Category: %v", err)
			}
		}
	})

	// ---- FindUnindexed ----
	b.Run("FindUnindexed/100", func(b *testing.B) {
		dir := b.TempDir()
		db, err := rainstormv5.Open(dir+"/bench.db", benchBoltOptsV5)
		if err != nil {
			b.Fatal(err)
		}
		b.Cleanup(func() {
			if cerr := db.Close(); cerr != nil {
				b.Errorf("close: %v", cerr)
			}
		})

		n := 100
		_ = populateRecordsV5(b, db, n)
		preflightReadV5(b, db, n)

		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			var dst []BenchmarkRecord
			if err := db.Find("Name", "name-a", &dst); err != nil {
				b.Fatalf("Find Name: %v", err)
			}
		}
	})

	b.Run("FindUnindexed/1000", func(b *testing.B) {
		dir := b.TempDir()
		db, err := rainstormv5.Open(dir+"/bench.db", benchBoltOptsV5)
		if err != nil {
			b.Fatal(err)
		}
		b.Cleanup(func() {
			if cerr := db.Close(); cerr != nil {
				b.Errorf("close: %v", cerr)
			}
		})

		n := 1000
		_ = populateRecordsV5(b, db, n)
		preflightReadV5(b, db, n)

		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			var dst []BenchmarkRecord
			if err := db.Find("Name", "name-a", &dst); err != nil {
				b.Fatalf("Find Name: %v", err)
			}
		}
	})

	// ---- All ----
	b.Run("All/100", func(b *testing.B) {
		dir := b.TempDir()
		db, err := rainstormv5.Open(dir+"/bench.db", benchBoltOptsV5)
		if err != nil {
			b.Fatal(err)
		}
		b.Cleanup(func() {
			if cerr := db.Close(); cerr != nil {
				b.Errorf("close: %v", cerr)
			}
		})

		n := 100
		_ = populateRecordsV5(b, db, n)
		preflightReadV5(b, db, n)

		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			var dst []BenchmarkRecord
			if err := db.All(&dst); err != nil {
				b.Fatalf("All: %v", err)
			}
		}
	})

	b.Run("All/1000", func(b *testing.B) {
		dir := b.TempDir()
		db, err := rainstormv5.Open(dir+"/bench.db", benchBoltOptsV5)
		if err != nil {
			b.Fatal(err)
		}
		b.Cleanup(func() {
			if cerr := db.Close(); cerr != nil {
				b.Errorf("close: %v", cerr)
			}
		})

		n := 1000
		_ = populateRecordsV5(b, db, n)
		preflightReadV5(b, db, n)

		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			var dst []BenchmarkRecord
			if err := db.All(&dst); err != nil {
				b.Fatalf("All: %v", err)
			}
		}
	})

	// ---- Save ----
	// Each iteration saves a new distinct record with a unique key.
	// The fmt.Sprintf call to generate the key is included in the timed
	// region identically for both v5 and v6.
	b.Run("Save", func(b *testing.B) {
		dir := b.TempDir()
		db, err := rainstormv5.Open(dir+"/bench.db", benchBoltOptsV5)
		if err != nil {
			b.Fatal(err)
		}
		b.Cleanup(func() {
			if cerr := db.Close(); cerr != nil {
				b.Errorf("close: %v", cerr)
			}
		})

		var last BenchmarkRecord
		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			last = BenchmarkRecord{
				Key:      fmt.Sprintf("save-%d", i),
				Category: "save-cat",
				Name:     "save-name",
				Revision: i,
				Payload:  payload100,
			}
			if err := db.Save(&last); err != nil {
				b.Fatalf("Save: %v", err)
			}
		}
		b.StopTimer()
		if last.ID == 0 {
			b.Fatal("Save did not assign an ID")
		}
		var persisted BenchmarkRecord
		if err := db.One("Key", last.Key, &persisted); err != nil {
			b.Fatalf("verify final Save: %v", err)
		}
		if persisted.ID != last.ID || persisted.Revision != last.Revision {
			b.Fatalf("final Save mismatch: got ID=%d revision=%d, want ID=%d revision=%d", persisted.ID, persisted.Revision, last.ID, last.Revision)
		}
	})

	// ---- Update ----
	b.Run("Update/100", func(b *testing.B) {
		dir := b.TempDir()
		db, err := rainstormv5.Open(dir+"/bench.db", benchBoltOptsV5)
		if err != nil {
			b.Fatal(err)
		}
		b.Cleanup(func() {
			if cerr := db.Close(); cerr != nil {
				b.Errorf("close: %v", cerr)
			}
		})

		n := 100
		records := populateRecordsV5(b, db, n)
		preflightReadV5(b, db, n)

		upd := records[n/2]

		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			upd.Revision = 1001 + i%2 // alternate between two non-zero values
			if err := db.Update(&upd); err != nil {
				b.Fatalf("Update: %v", err)
			}
		}
		b.StopTimer()
		var persisted BenchmarkRecord
		if err := db.One("ID", upd.ID, &persisted); err != nil {
			b.Fatalf("verify final Update: %v", err)
		}
		if persisted.ID != upd.ID || persisted.Revision != upd.Revision {
			b.Fatalf("final Update mismatch: got ID=%d revision=%d, want ID=%d revision=%d", persisted.ID, persisted.Revision, upd.ID, upd.Revision)
		}
	})

	b.Run("Update/1000", func(b *testing.B) {
		dir := b.TempDir()
		db, err := rainstormv5.Open(dir+"/bench.db", benchBoltOptsV5)
		if err != nil {
			b.Fatal(err)
		}
		b.Cleanup(func() {
			if cerr := db.Close(); cerr != nil {
				b.Errorf("close: %v", cerr)
			}
		})

		n := 1000
		records := populateRecordsV5(b, db, n)
		preflightReadV5(b, db, n)

		upd := records[n/2]

		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			upd.Revision = 1001 + i%2
			if err := db.Update(&upd); err != nil {
				b.Fatalf("Update: %v", err)
			}
		}
		b.StopTimer()
		var persisted BenchmarkRecord
		if err := db.One("ID", upd.ID, &persisted); err != nil {
			b.Fatalf("verify final Update: %v", err)
		}
		if persisted.ID != upd.ID || persisted.Revision != upd.Revision {
			b.Fatalf("final Update mismatch: got ID=%d revision=%d, want ID=%d revision=%d", persisted.ID, persisted.Revision, upd.ID, upd.Revision)
		}
	})

	// ---- KVGet ----
	b.Run("KVGet", func(b *testing.B) {
		dir := b.TempDir()
		db, err := rainstormv5.Open(dir+"/bench.db", benchBoltOptsV5)
		if err != nil {
			b.Fatal(err)
		}
		b.Cleanup(func() {
			if cerr := db.Close(); cerr != nil {
				b.Errorf("close: %v", cerr)
			}
		})

		if err := db.Set("bench_kv", "val", "hello"); err != nil {
			b.Fatalf("KV setup: %v", err)
		}
		var check string
		if err := db.Get("bench_kv", "val", &check); err != nil {
			b.Fatalf("KV preflight: %v", err)
		}
		if check != "hello" {
			b.Fatalf("KV preflight: want hello, got %s", check)
		}

		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			var dst string
			if err := db.Get("bench_kv", "val", &dst); err != nil {
				b.Fatalf("Get: %v", err)
			}
		}
	})

	// ---- KVSet ----
	// Measures overwrite of the same key.
	b.Run("KVSet", func(b *testing.B) {
		dir := b.TempDir()
		db, err := rainstormv5.Open(dir+"/bench.db", benchBoltOptsV5)
		if err != nil {
			b.Fatal(err)
		}
		b.Cleanup(func() {
			if cerr := db.Close(); cerr != nil {
				b.Errorf("close: %v", cerr)
			}
		})

		if err := db.Set("bench_kv", "val", "hello"); err != nil {
			b.Fatalf("KV setup: %v", err)
		}

		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			v := "hello"
			if i%2 == 1 {
				v = "world"
			}
			if err := db.Set("bench_kv", "val", v); err != nil {
				b.Fatalf("Set: %v", err)
			}
		}
	})
}

// =========================================================================
// V6 Benchmarks
// =========================================================================

func BenchmarkV6(b *testing.B) {
	ctx := context.Background()

	// ---- OneByID ----
	b.Run("OneByID/100", func(b *testing.B) {
		dir := b.TempDir()
		db, err := rainstormv6.Open(ctx, dir+"/bench.db", benchBoltOptsV6)
		if err != nil {
			b.Fatal(err)
		}
		b.Cleanup(func() {
			if cerr := db.Close(); cerr != nil {
				b.Errorf("close: %v", cerr)
			}
		})

		n := 100
		_ = populateRecordsV6(b, db, n)
		preflightReadV6(b, db, n)

		lookupID := uint64(n / 2)

		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			var dst BenchmarkRecord
			if err := db.One(ctx, "ID", lookupID, &dst); err != nil {
				b.Fatalf("One ID: %v", err)
			}
		}
	})

	b.Run("OneByID/1000", func(b *testing.B) {
		dir := b.TempDir()
		db, err := rainstormv6.Open(ctx, dir+"/bench.db", benchBoltOptsV6)
		if err != nil {
			b.Fatal(err)
		}
		b.Cleanup(func() {
			if cerr := db.Close(); cerr != nil {
				b.Errorf("close: %v", cerr)
			}
		})

		n := 1000
		_ = populateRecordsV6(b, db, n)
		preflightReadV6(b, db, n)

		lookupID := uint64(n / 2)

		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			var dst BenchmarkRecord
			if err := db.One(ctx, "ID", lookupID, &dst); err != nil {
				b.Fatalf("One ID: %v", err)
			}
		}
	})

	// ---- OneByUnique ----
	b.Run("OneByUnique/100", func(b *testing.B) {
		dir := b.TempDir()
		db, err := rainstormv6.Open(ctx, dir+"/bench.db", benchBoltOptsV6)
		if err != nil {
			b.Fatal(err)
		}
		b.Cleanup(func() {
			if cerr := db.Close(); cerr != nil {
				b.Errorf("close: %v", cerr)
			}
		})

		n := 100
		_ = populateRecordsV6(b, db, n)
		preflightReadV6(b, db, n)

		lookupKey := fmt.Sprintf("key-%04d", n/2)

		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			var dst BenchmarkRecord
			if err := db.One(ctx, "Key", lookupKey, &dst); err != nil {
				b.Fatalf("One Key: %v", err)
			}
		}
	})

	b.Run("OneByUnique/1000", func(b *testing.B) {
		dir := b.TempDir()
		db, err := rainstormv6.Open(ctx, dir+"/bench.db", benchBoltOptsV6)
		if err != nil {
			b.Fatal(err)
		}
		b.Cleanup(func() {
			if cerr := db.Close(); cerr != nil {
				b.Errorf("close: %v", cerr)
			}
		})

		n := 1000
		_ = populateRecordsV6(b, db, n)
		preflightReadV6(b, db, n)

		lookupKey := fmt.Sprintf("key-%04d", n/2)

		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			var dst BenchmarkRecord
			if err := db.One(ctx, "Key", lookupKey, &dst); err != nil {
				b.Fatalf("One Key: %v", err)
			}
		}
	})

	// ---- FindIndexed ----
	b.Run("FindIndexed/100", func(b *testing.B) {
		dir := b.TempDir()
		db, err := rainstormv6.Open(ctx, dir+"/bench.db", benchBoltOptsV6)
		if err != nil {
			b.Fatal(err)
		}
		b.Cleanup(func() {
			if cerr := db.Close(); cerr != nil {
				b.Errorf("close: %v", cerr)
			}
		})

		n := 100
		_ = populateRecordsV6(b, db, n)
		preflightReadV6(b, db, n)

		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			var dst []BenchmarkRecord
			if err := db.Find(ctx, "Category", "cat-a", &dst); err != nil {
				b.Fatalf("Find Category: %v", err)
			}
		}
	})

	b.Run("FindIndexed/1000", func(b *testing.B) {
		dir := b.TempDir()
		db, err := rainstormv6.Open(ctx, dir+"/bench.db", benchBoltOptsV6)
		if err != nil {
			b.Fatal(err)
		}
		b.Cleanup(func() {
			if cerr := db.Close(); cerr != nil {
				b.Errorf("close: %v", cerr)
			}
		})

		n := 1000
		_ = populateRecordsV6(b, db, n)
		preflightReadV6(b, db, n)

		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			var dst []BenchmarkRecord
			if err := db.Find(ctx, "Category", "cat-a", &dst); err != nil {
				b.Fatalf("Find Category: %v", err)
			}
		}
	})

	// ---- FindUnindexed ----
	b.Run("FindUnindexed/100", func(b *testing.B) {
		dir := b.TempDir()
		db, err := rainstormv6.Open(ctx, dir+"/bench.db", benchBoltOptsV6)
		if err != nil {
			b.Fatal(err)
		}
		b.Cleanup(func() {
			if cerr := db.Close(); cerr != nil {
				b.Errorf("close: %v", cerr)
			}
		})

		n := 100
		_ = populateRecordsV6(b, db, n)
		preflightReadV6(b, db, n)

		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			var dst []BenchmarkRecord
			if err := db.Find(ctx, "Name", "name-a", &dst); err != nil {
				b.Fatalf("Find Name: %v", err)
			}
		}
	})

	b.Run("FindUnindexed/1000", func(b *testing.B) {
		dir := b.TempDir()
		db, err := rainstormv6.Open(ctx, dir+"/bench.db", benchBoltOptsV6)
		if err != nil {
			b.Fatal(err)
		}
		b.Cleanup(func() {
			if cerr := db.Close(); cerr != nil {
				b.Errorf("close: %v", cerr)
			}
		})

		n := 1000
		_ = populateRecordsV6(b, db, n)
		preflightReadV6(b, db, n)

		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			var dst []BenchmarkRecord
			if err := db.Find(ctx, "Name", "name-a", &dst); err != nil {
				b.Fatalf("Find Name: %v", err)
			}
		}
	})

	// ---- All ----
	b.Run("All/100", func(b *testing.B) {
		dir := b.TempDir()
		db, err := rainstormv6.Open(ctx, dir+"/bench.db", benchBoltOptsV6)
		if err != nil {
			b.Fatal(err)
		}
		b.Cleanup(func() {
			if cerr := db.Close(); cerr != nil {
				b.Errorf("close: %v", cerr)
			}
		})

		n := 100
		_ = populateRecordsV6(b, db, n)
		preflightReadV6(b, db, n)

		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			var dst []BenchmarkRecord
			if err := db.All(ctx, &dst); err != nil {
				b.Fatalf("All: %v", err)
			}
		}
	})

	b.Run("All/1000", func(b *testing.B) {
		dir := b.TempDir()
		db, err := rainstormv6.Open(ctx, dir+"/bench.db", benchBoltOptsV6)
		if err != nil {
			b.Fatal(err)
		}
		b.Cleanup(func() {
			if cerr := db.Close(); cerr != nil {
				b.Errorf("close: %v", cerr)
			}
		})

		n := 1000
		_ = populateRecordsV6(b, db, n)
		preflightReadV6(b, db, n)

		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			var dst []BenchmarkRecord
			if err := db.All(ctx, &dst); err != nil {
				b.Fatalf("All: %v", err)
			}
		}
	})

	// ---- Save ----
	// Each iteration saves a new distinct record with a unique key.
	// The fmt.Sprintf call to generate the key is included in the timed
	// region identically for both v5 and v6.
	b.Run("Save", func(b *testing.B) {
		dir := b.TempDir()
		db, err := rainstormv6.Open(ctx, dir+"/bench.db", benchBoltOptsV6)
		if err != nil {
			b.Fatal(err)
		}
		b.Cleanup(func() {
			if cerr := db.Close(); cerr != nil {
				b.Errorf("close: %v", cerr)
			}
		})

		var last BenchmarkRecord
		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			last = BenchmarkRecord{
				Key:      fmt.Sprintf("save-%d", i),
				Category: "save-cat",
				Name:     "save-name",
				Revision: i,
				Payload:  payload100,
			}
			if err := db.Save(ctx, &last); err != nil {
				b.Fatalf("Save: %v", err)
			}
		}
		b.StopTimer()
		if last.ID == 0 {
			b.Fatal("Save did not assign an ID")
		}
		var persisted BenchmarkRecord
		if err := db.One(ctx, "Key", last.Key, &persisted); err != nil {
			b.Fatalf("verify final Save: %v", err)
		}
		if persisted.ID != last.ID || persisted.Revision != last.Revision {
			b.Fatalf("final Save mismatch: got ID=%d revision=%d, want ID=%d revision=%d", persisted.ID, persisted.Revision, last.ID, last.Revision)
		}
	})

	// ---- Update ----
	b.Run("Update/100", func(b *testing.B) {
		dir := b.TempDir()
		db, err := rainstormv6.Open(ctx, dir+"/bench.db", benchBoltOptsV6)
		if err != nil {
			b.Fatal(err)
		}
		b.Cleanup(func() {
			if cerr := db.Close(); cerr != nil {
				b.Errorf("close: %v", cerr)
			}
		})

		n := 100
		records := populateRecordsV6(b, db, n)
		preflightReadV6(b, db, n)

		upd := records[n/2]

		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			upd.Revision = 1001 + i%2
			if err := db.Update(ctx, &upd); err != nil {
				b.Fatalf("Update: %v", err)
			}
		}
		b.StopTimer()
		var persisted BenchmarkRecord
		if err := db.One(ctx, "ID", upd.ID, &persisted); err != nil {
			b.Fatalf("verify final Update: %v", err)
		}
		if persisted.ID != upd.ID || persisted.Revision != upd.Revision {
			b.Fatalf("final Update mismatch: got ID=%d revision=%d, want ID=%d revision=%d", persisted.ID, persisted.Revision, upd.ID, upd.Revision)
		}
	})

	b.Run("Update/1000", func(b *testing.B) {
		dir := b.TempDir()
		db, err := rainstormv6.Open(ctx, dir+"/bench.db", benchBoltOptsV6)
		if err != nil {
			b.Fatal(err)
		}
		b.Cleanup(func() {
			if cerr := db.Close(); cerr != nil {
				b.Errorf("close: %v", cerr)
			}
		})

		n := 1000
		records := populateRecordsV6(b, db, n)
		preflightReadV6(b, db, n)

		upd := records[n/2]

		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			upd.Revision = 1001 + i%2
			if err := db.Update(ctx, &upd); err != nil {
				b.Fatalf("Update: %v", err)
			}
		}
		b.StopTimer()
		var persisted BenchmarkRecord
		if err := db.One(ctx, "ID", upd.ID, &persisted); err != nil {
			b.Fatalf("verify final Update: %v", err)
		}
		if persisted.ID != upd.ID || persisted.Revision != upd.Revision {
			b.Fatalf("final Update mismatch: got ID=%d revision=%d, want ID=%d revision=%d", persisted.ID, persisted.Revision, upd.ID, upd.Revision)
		}
	})

	// ---- KVGet ----
	b.Run("KVGet", func(b *testing.B) {
		dir := b.TempDir()
		db, err := rainstormv6.Open(ctx, dir+"/bench.db", benchBoltOptsV6)
		if err != nil {
			b.Fatal(err)
		}
		b.Cleanup(func() {
			if cerr := db.Close(); cerr != nil {
				b.Errorf("close: %v", cerr)
			}
		})

		if err := db.Set(ctx, "bench_kv", "val", "hello"); err != nil {
			b.Fatalf("KV setup: %v", err)
		}
		var check string
		if err := db.Get(ctx, "bench_kv", "val", &check); err != nil {
			b.Fatalf("KV preflight: %v", err)
		}
		if check != "hello" {
			b.Fatalf("KV preflight: want hello, got %s", check)
		}

		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			var dst string
			if err := db.Get(ctx, "bench_kv", "val", &dst); err != nil {
				b.Fatalf("Get: %v", err)
			}
		}
	})

	// ---- KVSet ----
	// Measures overwrite of the same key.
	b.Run("KVSet", func(b *testing.B) {
		dir := b.TempDir()
		db, err := rainstormv6.Open(ctx, dir+"/bench.db", benchBoltOptsV6)
		if err != nil {
			b.Fatal(err)
		}
		b.Cleanup(func() {
			if cerr := db.Close(); cerr != nil {
				b.Errorf("close: %v", cerr)
			}
		})

		if err := db.Set(ctx, "bench_kv", "val", "hello"); err != nil {
			b.Fatalf("KV setup: %v", err)
		}

		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			v := "hello"
			if i%2 == 1 {
				v = "world"
			}
			if err := db.Set(ctx, "bench_kv", "val", v); err != nil {
				b.Fatalf("Set: %v", err)
			}
		}
	})
}

// =========================================================================
// V6-only Benchmarks
// =========================================================================

// BenchmarkV6_ReadTransaction groups multiple reads (One + Find + All + Get)
// within a single ReadTransaction to measure transaction overhead and
// savings from reusing one bbolt read transaction.
func BenchmarkV6_ReadTransaction(b *testing.B) {
	ctx := context.Background()

	dir := b.TempDir()
	db, err := rainstormv6.Open(ctx, dir+"/bench.db", benchBoltOptsV6)
	if err != nil {
		b.Fatal(err)
	}
	b.Cleanup(func() {
		if cerr := db.Close(); cerr != nil {
			b.Errorf("close: %v", cerr)
		}
	})

	n := 100
	_ = populateRecordsV6(b, db, n)
	preflightReadV6(b, db, n)

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		err := db.ReadTransaction(ctx, func(tx rainstormv6.Node) error {
			var one BenchmarkRecord
			if err := tx.One(ctx, "ID", uint64(1), &one); err != nil {
				return err
			}

			var indexed []BenchmarkRecord
			if err := tx.Find(ctx, "Category", "cat-a", &indexed); err != nil {
				return err
			}

			var all []BenchmarkRecord
			if err := tx.All(ctx, &all); err != nil {
				return err
			}

			var kvVal string
			if err := tx.Get(ctx, "bench_kv", "val", &kvVal); err != nil {
				return err
			}
			return nil
		})
		if err != nil {
			b.Fatalf("ReadTransaction: %v", err)
		}
	}
}

// BenchmarkV6_WriteTransaction groups multiple writes (Set calls) within
// a single WriteTransaction to measure write amortisation overhead.
func BenchmarkV6_WriteTransaction(b *testing.B) {
	ctx := context.Background()

	dir := b.TempDir()
	db, err := rainstormv6.Open(ctx, dir+"/bench.db", benchBoltOptsV6)
	if err != nil {
		b.Fatal(err)
	}
	b.Cleanup(func() {
		if cerr := db.Close(); cerr != nil {
			b.Errorf("close: %v", cerr)
		}
	})

	if err := db.Set(ctx, "bench_kv", "val", "hello"); err != nil {
		b.Fatalf("KV setup: %v", err)
	}

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		n := i
		err := db.WriteTransaction(ctx, func(tx rainstormv6.Node) error {
			if err := tx.Set(ctx, "bench_kv", "val", fmt.Sprintf("tx-val-%d", n)); err != nil {
				return err
			}
			if err := tx.Set(ctx, "bench_kv", "seq", n); err != nil {
				return err
			}
			return nil
		})
		if err != nil {
			b.Fatalf("WriteTransaction: %v", err)
		}
	}
}

// BenchmarkV6_CanceledContext measures the cost of a canceled-context
// fast rejection path — how quickly an operation returns when the
// context is already canceled before the call.
func BenchmarkV6_CanceledContext(b *testing.B) {
	ctx := context.Background()

	dir := b.TempDir()
	db, err := rainstormv6.Open(ctx, dir+"/bench.db", benchBoltOptsV6)
	if err != nil {
		b.Fatal(err)
	}
	b.Cleanup(func() {
		if cerr := db.Close(); cerr != nil {
			b.Errorf("close: %v", cerr)
		}
	})

	// Set up minimal data.
	if err := db.Set(ctx, "bench_kv", "val", "hello"); err != nil {
		b.Fatalf("setup: %v", err)
	}

	cancelCtx, cancel := context.WithCancel(ctx)
	cancel()

	var preflightDst string
	if err := db.Get(cancelCtx, "bench_kv", "val", &preflightDst); !errors.Is(err, context.Canceled) {
		b.Fatalf("canceled-context preflight: expected context.Canceled, got %v", err)
	}

	var lastErr error
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		var dst string
		lastErr = db.Get(cancelCtx, "bench_kv", "val", &dst)
	}
	b.StopTimer()
	if !errors.Is(lastErr, context.Canceled) {
		b.Fatalf("canceled-context result: expected context.Canceled, got %v", lastErr)
	}
}
