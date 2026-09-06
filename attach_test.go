package rainstorm

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/AndersonBargas/rainstorm/v6/q"
	"github.com/stretchr/testify/require"
	bolt "go.etcd.io/bbolt"
	bolterrors "go.etcd.io/bbolt/errors"
)

// attachUser exercises the record, index, and unique-index paths.
type attachUser struct {
	ID   int    `rainstorm:"id"`
	Name string `rainstorm:"index"`
	Tag  string `rainstorm:"unique"`
}

// attachDB opens a Rainstorm database in a temporary directory.
// Cleanup closes the database; register it before any transaction cleanup so
// that cleanup LIFO order rolls transactions back before the database closes.
func attachDB(t *testing.T) *DB {
	t.Helper()

	path := filepath.Join(t.TempDir(), "rainstorm.db")
	db, err := Open(context.Background(), path)
	require.NoError(t, err)
	t.Cleanup(func() {
		require.NoError(t, db.Close())
	})
	return db
}

// attachBegin opens a caller-owned bbolt transaction on db.
// Cleanup rolls the transaction back if the test ends before committing, so a
// failing assertion cannot leak an open transaction into db.Close.
func attachBegin(t *testing.T, db *DB, writable bool) *bolt.Tx {
	t.Helper()

	tx, err := db.NativeDB().Begin(writable)
	require.NoError(t, err)
	t.Cleanup(func() {
		// No-op when the test already committed or rolled back.
		_ = tx.Rollback()
	})
	return tx
}

// ---------------------------------------------------------------------------
// 1. Rainstorm record write + raw bolt bucket write commit atomically
// ---------------------------------------------------------------------------

func TestAttach_CommitIsAtomic(t *testing.T) {
	db := attachDB(t)
	ctx := context.Background()

	tx := attachBegin(t, db, true)
	txn, err := db.Attach(tx)
	require.NoError(t, err)

	// Rainstorm record write (record + ID/index/unique/metadata buckets).
	err = txn.Save(ctx, &attachUser{ID: 1, Name: "alice", Tag: "t1"})
	require.NoError(t, err)

	// Raw bolt write in the same caller-owned transaction.
	audit, err := tx.CreateBucketIfNotExists([]byte("audit"))
	require.NoError(t, err)
	require.NoError(t, audit.Put([]byte("event"), []byte("created")))

	// Nothing is visible before the caller commits.
	require.NoError(t, tx.Commit())

	// The record is visible through normal Rainstorm reads.
	var u attachUser
	require.NoError(t, db.One(ctx, "ID", 1, &u))
	require.Equal(t, "alice", u.Name)

	// The raw bolt write committed atomically with the Rainstorm write.
	var raw []byte
	require.NoError(t, db.NativeDB().View(func(vtx *bolt.Tx) error {
		b := vtx.Bucket([]byte("audit"))
		if b == nil {
			return ErrNotFound
		}
		v := b.Get([]byte("event"))
		if v == nil {
			return ErrNotFound
		}
		raw = append([]byte(nil), v...)
		return nil
	}))
	require.Equal(t, "created", string(raw))

	// The handle died with the transaction: a post-commit write fails with
	// bbolt's closed-transaction error instead of opening a managed one.
	err = txn.Save(ctx, &attachUser{ID: 2, Name: "bob", Tag: "t2"})
	require.ErrorIs(t, err, bolterrors.ErrTxClosed)
}

// ---------------------------------------------------------------------------
// 2. Failure before caller commit rolls both back
// ---------------------------------------------------------------------------

func TestAttach_RollbackDiscardsEverything(t *testing.T) {
	db := attachDB(t)
	ctx := context.Background()

	tx := attachBegin(t, db, true)
	txn, err := db.Attach(tx)
	require.NoError(t, err)

	// Rainstorm record write plus a raw bolt write in the same transaction.
	require.NoError(t, txn.Save(ctx, &attachUser{ID: 1, Name: "alice", Tag: "t1"}))

	audit, err := tx.CreateBucketIfNotExists([]byte("audit"))
	require.NoError(t, err)
	require.NoError(t, audit.Put([]byte("event"), []byte("created")))

	// The caller aborts (failure path) before committing.
	require.NoError(t, tx.Rollback())

	// The record is gone.
	var u attachUser
	require.ErrorIs(t, db.One(ctx, "ID", 1, &u), ErrNotFound)

	// The index entries are gone.
	var found []attachUser
	require.ErrorIs(t, db.Find(ctx, "Name", "alice", &found), ErrNotFound)
	require.ErrorIs(t, db.Find(ctx, "Tag", "t1", &found), ErrNotFound)

	// The raw bolt write is gone too.
	require.NoError(t, db.NativeDB().View(func(vtx *bolt.Tx) error {
		require.Nil(t, vtx.Bucket([]byte("audit")))
		return nil
	}))

	// No unique-index residue: the same ID and Tag are reusable.
	require.NoError(t, db.Save(ctx, &attachUser{ID: 1, Name: "alice", Tag: "t1"}))
}

// ---------------------------------------------------------------------------
// 3. Uncommitted writes are visible to reads through the same bound handle
// ---------------------------------------------------------------------------

func TestAttach_ReadsOwnUncommittedWrites(t *testing.T) {
	db := attachDB(t)
	ctx := context.Background()

	tx := attachBegin(t, db, true)
	txn, err := db.Attach(tx)
	require.NoError(t, err)

	require.NoError(t, txn.Save(ctx, &attachUser{ID: 1, Name: "alice", Tag: "t1"}))

	// Record reads see the uncommitted write.
	var u attachUser
	require.NoError(t, txn.One(ctx, "ID", 1, &u))
	require.Equal(t, "alice", u.Name)

	// Index reads see the uncommitted write.
	var byName []attachUser
	require.NoError(t, txn.Find(ctx, "Name", "alice", &byName))
	require.Len(t, byName, 1)
	require.Equal(t, "alice", byName[0].Name)

	var byTag []attachUser
	require.NoError(t, txn.Find(ctx, "Tag", "t1", &byTag))
	require.Len(t, byTag, 1)

	// Full scans see the uncommitted write.
	var all []attachUser
	require.NoError(t, txn.All(ctx, &all))
	require.Len(t, all, 1)

	// Queries see the uncommitted write.
	var queried []attachUser
	require.NoError(t, txn.Select(q.Eq("Name", "alice")).Find(ctx, &queried))
	require.Len(t, queried, 1)

	// Key/value access sees the uncommitted write.
	require.NoError(t, txn.Set(ctx, "kv", "k", "v"))
	var v string
	require.NoError(t, txn.Get(ctx, "kv", "k", &v))
	require.Equal(t, "v", v)

	// Derived nodes keep the binding.
	nested := txn.From("nested")
	require.NoError(t, nested.Save(ctx, &attachUser{ID: 2, Name: "nested", Tag: "n1"}))
	var nestedRead attachUser
	require.NoError(t, nested.One(ctx, "ID", 2, &nestedRead))
	require.Equal(t, "nested", nestedRead.Name)

	require.NoError(t, tx.Rollback())

	// Everything is discarded after the caller rolls back.
	require.ErrorIs(t, db.One(ctx, "ID", 1, &u), ErrNotFound)
	var gone []attachUser
	require.ErrorIs(t, db.Find(ctx, "Name", "alice", &gone), ErrNotFound)
}

// ---------------------------------------------------------------------------
// 4. Uncommitted writes are invisible from other transactions
// ---------------------------------------------------------------------------

func TestAttach_IsolationFromOtherTransactions(t *testing.T) {
	db := attachDB(t)
	ctx := context.Background()

	tx1 := attachBegin(t, db, true)
	txn1, err := db.Attach(tx1)
	require.NoError(t, err)

	require.NoError(t, txn1.Save(ctx, &attachUser{ID: 1, Name: "alice", Tag: "t1"}))

	// A raw bolt reader does not see the uncommitted write.
	require.NoError(t, db.NativeDB().View(func(vtx *bolt.Tx) error {
		require.Nil(t, vtx.Bucket([]byte("attachUser")))
		return nil
	}))

	// A managed Rainstorm read does not see the uncommitted write.
	var u attachUser
	require.ErrorIs(t, db.One(ctx, "ID", 1, &u), ErrNotFound)

	// A second attached read-only transaction does not see it either.
	tx2 := attachBegin(t, db, false)
	txn2, err := db.Attach(tx2)
	require.NoError(t, err)
	require.ErrorIs(t, txn2.One(ctx, "ID", 1, &u), ErrNotFound)
	require.NoError(t, tx2.Rollback())

	// The writing handle still sees its own uncommitted write.
	require.NoError(t, txn1.One(ctx, "ID", 1, &u))
	require.Equal(t, "alice", u.Name)

	// Only after the caller commits does the write become visible.
	require.NoError(t, tx1.Commit())
	require.NoError(t, db.One(ctx, "ID", 1, &u))
	require.Equal(t, "alice", u.Name)
}

// ---------------------------------------------------------------------------
// 5. Rainstorm never commits, rolls back, or closes a caller-owned transaction
// ---------------------------------------------------------------------------

func TestAttach_NeverCommitsOrRollsBackCallerTx(t *testing.T) {
	db := attachDB(t)
	ctx := context.Background()

	tx := attachBegin(t, db, true)
	txn, err := db.Attach(tx)
	require.NoError(t, err)

	require.NoError(t, txn.Save(ctx, &attachUser{ID: 1, Name: "alice", Tag: "t1"}))

	// The transaction must still be open and writable: a raw bolt write
	// succeeds only if Rainstorm neither committed nor rolled it back.
	audit, err := tx.CreateBucketIfNotExists([]byte("audit"))
	require.NoError(t, err)
	require.NoError(t, audit.Put([]byte("event"), []byte("created")))

	// Nothing was committed in between.
	require.NoError(t, db.NativeDB().View(func(vtx *bolt.Tx) error {
		require.Nil(t, vtx.Bucket([]byte("attachUser")))
		require.Nil(t, vtx.Bucket([]byte("audit")))
		return nil
	}))

	// The handle remains usable for further writes without reopening the tx.
	require.NoError(t, txn.Save(ctx, &attachUser{ID: 2, Name: "bob", Tag: "t2"}))

	// The caller is still the one who decides the outcome.
	require.NoError(t, tx.Commit())

	var u attachUser
	require.NoError(t, db.One(ctx, "ID", 1, &u))
	require.NoError(t, db.One(ctx, "ID", 2, &u))
}

func TestAttach_ReadOnlyTransactionWritesFailWithBboltError(t *testing.T) {
	db := attachDB(t)
	ctx := context.Background()

	// Seed committed data for the read side of this test.
	require.NoError(t, db.Save(ctx, &attachUser{ID: 1, Name: "alice", Tag: "t1"}))

	tx := attachBegin(t, db, false)
	txn, err := db.Attach(tx)
	require.NoError(t, err)

	// Reads through a read-only transaction work.
	var u attachUser
	require.NoError(t, txn.One(ctx, "ID", 1, &u))
	require.Equal(t, "alice", u.Name)

	// Writes fail with bbolt's not-writable sentinel. This also proves the
	// write targeted the caller's transaction: a managed transaction would
	// have accepted it.
	err = txn.Save(ctx, &attachUser{ID: 2, Name: "bob", Tag: "t2"})
	require.ErrorIs(t, err, bolterrors.ErrTxNotWritable)

	require.NoError(t, tx.Rollback())
}

// ---------------------------------------------------------------------------
// 6. Indexes remain consistent under commit and rollback
// ---------------------------------------------------------------------------

func TestAttach_IndexConsistencyOnCommit(t *testing.T) {
	db := attachDB(t)
	ctx := context.Background()

	// Commit path: index entries created inside the transaction survive it.
	tx1 := attachBegin(t, db, true)
	txn1, err := db.Attach(tx1)
	require.NoError(t, err)
	require.NoError(t, txn1.Save(ctx, &attachUser{ID: 1, Name: "alice", Tag: "t1"}))
	require.NoError(t, tx1.Commit())

	var found []attachUser
	require.NoError(t, db.Find(ctx, "Name", "alice", &found))
	require.Len(t, found, 1)

	require.NoError(t, db.AllByIndex(ctx, "Name", &found))
	require.Len(t, found, 1)

	// Updating an indexed field inside a transaction migrates the index
	// entries atomically on commit.
	tx2 := attachBegin(t, db, true)
	txn2, err := db.Attach(tx2)
	require.NoError(t, err)

	var u attachUser
	require.NoError(t, txn2.One(ctx, "ID", 1, &u))
	u.Name = "bob"
	require.NoError(t, txn2.Update(ctx, &u))
	require.NoError(t, tx2.Commit())

	require.ErrorIs(t, db.Find(ctx, "Name", "alice", &found), ErrNotFound)
	require.NoError(t, db.Find(ctx, "Name", "bob", &found))
	require.Len(t, found, 1)
}

func TestAttach_IndexConsistencyOnRollback(t *testing.T) {
	db := attachDB(t)
	ctx := context.Background()

	// Rollback path: index entries created inside the transaction disappear.
	tx1 := attachBegin(t, db, true)
	txn1, err := db.Attach(tx1)
	require.NoError(t, err)
	require.NoError(t, txn1.Save(ctx, &attachUser{ID: 2, Name: "carol", Tag: "t2"}))
	require.NoError(t, tx1.Rollback())

	var found []attachUser
	require.ErrorIs(t, db.Find(ctx, "Name", "carol", &found), ErrNotFound)
	require.ErrorIs(t, db.Find(ctx, "Tag", "t2", &found), ErrNotFound)

	// No unique-index residue: another record may reuse the Tag.
	require.NoError(t, db.Save(ctx, &attachUser{ID: 3, Name: "dave", Tag: "t2"}))

	// Rollback of an Update restores the previous index entries.
	require.NoError(t, db.Save(ctx, &attachUser{ID: 4, Name: "eve", Tag: "t3"}))

	tx2 := attachBegin(t, db, true)
	txn2, err := db.Attach(tx2)
	require.NoError(t, err)

	var u attachUser
	require.NoError(t, txn2.One(ctx, "ID", 4, &u))
	u.Name = "frank"
	require.NoError(t, txn2.Update(ctx, &u))
	require.NoError(t, tx2.Rollback())

	require.NoError(t, db.Find(ctx, "Name", "eve", &found))
	require.Len(t, found, 1)
	require.ErrorIs(t, db.Find(ctx, "Name", "frank", &found), ErrNotFound)
}

// ---------------------------------------------------------------------------
// 7. Attach validation and handle lifetime
// ---------------------------------------------------------------------------

func TestAttach_Validation(t *testing.T) {
	db := attachDB(t)

	// A nil transaction is rejected without panicking.
	txn, err := db.Attach(nil)
	require.ErrorIs(t, err, ErrNilParam)
	require.Nil(t, txn)

	// A nil receiver is rejected without panicking.
	var nilDB *DB
	txn, err = nilDB.Attach(nil)
	require.ErrorIs(t, err, ErrNilParam)
	require.Nil(t, txn)

	// A transaction from a different database is rejected.
	otherPath := filepath.Join(t.TempDir(), "other.db")
	otherDB, err := bolt.Open(otherPath, 0600, &bolt.Options{Timeout: 10 * time.Second})
	require.NoError(t, err)
	t.Cleanup(func() {
		require.NoError(t, otherDB.Close())
	})

	foreignTx, err := otherDB.Begin(true)
	require.NoError(t, err)
	t.Cleanup(func() {
		_ = foreignTx.Rollback()
	})

	txn, err = db.Attach(foreignTx)
	require.ErrorIs(t, err, ErrTxFromDifferentDB)
	require.Nil(t, txn)
}

func TestAttach_HandleDiesWithTransaction(t *testing.T) {
	db := attachDB(t)
	ctx := context.Background()

	// A handle is valid only for the lifetime of its transaction. After the
	// caller rolls back, writes through the dead handle fail with bbolt's
	// ErrTxClosed (bbolt checks the transaction before mutating) — they must
	// not fall back to a managed transaction that would silently succeed.
	// Reads through a dead handle are bbolt undefined behavior (freed pages)
	// and are intentionally not exercised here.
	tx := attachBegin(t, db, true)
	txn, err := db.Attach(tx)
	require.NoError(t, err)

	require.NoError(t, txn.Save(ctx, &attachUser{ID: 1, Name: "alice", Tag: "t1"}))
	require.NoError(t, tx.Rollback())

	err = txn.Save(ctx, &attachUser{ID: 2, Name: "bob", Tag: "t2"})
	require.ErrorIs(t, err, bolterrors.ErrTxClosed)
}
