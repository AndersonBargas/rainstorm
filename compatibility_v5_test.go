package rainstorm

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// CompatibilityUser matches the schema used by the v5.3.0 generator.
// Tags follow the v5.3.0 syntax verified against the v5.3.0 extract.go.
type CompatibilityUser struct {
	ID       uint64 `rainstorm:"id,increment"`
	Email    string `rainstorm:"unique"`
	Team     string `rainstorm:"index"`
	Name     string
	Revision int
}

// ------------- manifest types for test A -------------

type compatManifest struct {
	Source      compatSource    `json:"source"`
	Codec       string          `json:"codec"`
	RootRecords []compatRootRec `json:"root_records"`
	Indexes     compatIndexes   `json:"indexes"`
	Nested      compatNested    `json:"nested"`
	KV          compatKVSection `json:"kv"`
}

type compatSource struct {
	Module        string `json:"module"`
	Version       string `json:"version"`
	GeneratorPath string `json:"generator_path"`
	FixtureFile   string `json:"fixture_filename"`
}

type compatRootRec struct {
	ID       uint64 `json:"id"`
	Email    string `json:"email"`
	Team     string `json:"team"`
	Name     string `json:"name"`
	Revision int    `json:"revision"`
}

type compatIndexes struct {
	Team  compatListIdx   `json:"team"`
	Email compatUniqueIdx `json:"email"`
}

type compatListIdx struct {
	Type    string                   `json:"type"`
	Members map[string][]compatIDRef `json:"members"`
	Order   string                   `json:"order"`
}

type compatIDRef struct {
	ID    uint64 `json:"id"`
	Email string `json:"email"`
}

type compatUniqueIdx struct {
	Type    string            `json:"type"`
	Lookups map[string]uint64 `json:"lookups"`
}

type compatNested struct {
	Path    []string        `json:"path"`
	Records []compatRootRec `json:"records"`
}

type compatKVSection struct {
	Root   compatKVBucket `json:"root"`
	Nested compatKVBucket `json:"nested"`
}

type compatKVBucket struct {
	Bucket  string          `json:"bucket"`
	Entries []compatKVEntry `json:"entries"`
	Path    []string        `json:"path,omitempty"`
}

type compatKVEntry struct {
	Key   string      `json:"key"`
	Value interface{} `json:"value"`
}

// ------------- helpers -------------

// fixturePath returns the package-relative path to baseline.db.
func fixturePath(tb testing.TB) string {
	tb.Helper()
	// testdata/compatibility/v5.3.0/baseline.db relative to the package directory.
	return filepath.Join("testdata", "compatibility", "v5.3.0", "baseline.db")
}

// manifestPath returns the package-relative path to manifest.json.
func manifestPath(tb testing.TB) string {
	tb.Helper()
	return filepath.Join("testdata", "compatibility", "v5.3.0", "manifest.json")
}

// copyFixture copies baseline.db to a temp directory and returns the copy path.
func copyFixture(tb testing.TB) string {
	tb.Helper()
	src := fixturePath(tb)
	data, err := os.ReadFile(src)
	require.NoError(tb, err, "read fixture %s", src)

	dst := filepath.Join(tb.TempDir(), "baseline.db")
	err = os.WriteFile(dst, data, 0600)
	require.NoError(tb, err, "write copy to %s", dst)
	return dst
}

// openFixture opens a copied fixture with v6.
func openFixture(tb testing.TB, path string) *DB {
	tb.Helper()
	db, err := Open(context.Background(), path)
	require.NoError(tb, err, "open fixture %s", path)
	return db
}

// assertClose wraps db.Close() and reports failure through tb.
func assertClose(tb testing.TB, db *DB) {
	tb.Helper()
	require.NoError(tb, db.Close(), "close database")
}

// readManifest reads and decodes the manifest.
func readManifest(tb testing.TB) compatManifest {
	tb.Helper()
	data, err := os.ReadFile(manifestPath(tb))
	require.NoError(tb, err, "read manifest")

	var m compatManifest
	err = json.Unmarshal(data, &m)
	require.NoError(tb, err, "decode manifest")
	return m
}

// ------------- A. Provenance/manifest test -------------

func TestCompatibility_Manifest(t *testing.T) {
	m := readManifest(t)

	assert.Equal(t, "github.com/AndersonBargas/rainstorm", m.Source.Module)
	assert.Equal(t, "v5.3.0", m.Source.Version)
	assert.Equal(t, "baseline.db", m.Source.FixtureFile)
	assert.NotEmpty(t, m.Source.GeneratorPath)

	assert.Len(t, m.RootRecords, 4)
	emails := make(map[uint64]string)
	for _, r := range m.RootRecords {
		emails[r.ID] = r.Email
	}
	assert.Equal(t, "alice@example.test", emails[1])
	assert.Equal(t, "bob@example.test", emails[2])
	assert.Equal(t, "carol@example.test", emails[3])
	assert.Equal(t, "dave@example.test", emails[4])
}

// ------------- B. Open and baseline read -------------

func TestCompatibility_OpenBaselineRead(t *testing.T) {
	path := copyFixture(t)
	db := openFixture(t, path)
	defer assertClose(t, db)

	ctx := context.Background()

	// Read all four root records via All.
	var all []CompatibilityUser
	err := db.All(ctx, &all)
	require.NoError(t, err)
	require.Len(t, all, 4)

	// Build lookup by ID.
	byID := make(map[uint64]CompatibilityUser, 4)
	for _, u := range all {
		byID[u.ID] = u
	}

	// Verify each record matches the manifest exactly.
	assert.Equal(t, uint64(1), byID[1].ID)
	assert.Equal(t, "alice@example.test", byID[1].Email)
	assert.Equal(t, "team-blue", byID[1].Team)
	assert.Equal(t, "Alice", byID[1].Name)
	assert.Equal(t, 1, byID[1].Revision)

	assert.Equal(t, uint64(2), byID[2].ID)
	assert.Equal(t, "bob@example.test", byID[2].Email)
	assert.Equal(t, "team-blue", byID[2].Team)
	assert.Equal(t, "Bob", byID[2].Name)
	assert.Equal(t, 1, byID[2].Revision)

	assert.Equal(t, uint64(3), byID[3].ID)
	assert.Equal(t, "carol@example.test", byID[3].Email)
	assert.Equal(t, "team-red", byID[3].Team)
	assert.Equal(t, "Carol", byID[3].Name)
	assert.Equal(t, 2, byID[3].Revision)

	assert.Equal(t, uint64(4), byID[4].ID)
	assert.Equal(t, "dave@example.test", byID[4].Email)
	assert.Equal(t, "team-green", byID[4].Team)
	assert.Equal(t, "Dave", byID[4].Name)
	assert.Equal(t, 3, byID[4].Revision)
}

// ------------- C. Increment ID compatibility -------------

func TestCompatibility_IncrementID(t *testing.T) {
	path := copyFixture(t)
	db := openFixture(t, path)
	defer assertClose(t, db)

	ctx := context.Background()

	// Read records by ID to prove v5 IDs are accepted by v6.
	var u1 CompatibilityUser
	err := db.One(ctx, "ID", uint64(1), &u1)
	require.NoError(t, err)
	assert.Equal(t, "alice@example.test", u1.Email)

	var u4 CompatibilityUser
	err = db.One(ctx, "ID", uint64(4), &u4)
	require.NoError(t, err)
	assert.Equal(t, "dave@example.test", u4.Email)

	// Save a new record and verify its ID continues after max v5 ID (4).
	newUser := CompatibilityUser{
		Email:    "new@example.test",
		Team:     "team-new",
		Name:     "New User",
		Revision: 10,
	}
	err = db.Save(ctx, &newUser)
	require.NoError(t, err)
	assert.Equal(t, uint64(5), newUser.ID, "new ID should continue after highest v5 ID (4)")

	// Read it back.
	var fetched CompatibilityUser
	err = db.One(ctx, "ID", newUser.ID, &fetched)
	require.NoError(t, err)
	assert.Equal(t, "new@example.test", fetched.Email)
	assert.Equal(t, "team-new", fetched.Team)

	// Existing records remain unchanged.
	var u2 CompatibilityUser
	err = db.One(ctx, "ID", uint64(2), &u2)
	require.NoError(t, err)
	assert.Equal(t, "bob@example.test", u2.Email)
}

// ------------- D. Ordinary index compatibility -------------

func TestCompatibility_OrdinaryIndex(t *testing.T) {
	path := copyFixture(t)
	db := openFixture(t, path)
	defer assertClose(t, db)

	ctx := context.Background()

	// Team == team-blue returns exactly Alice then Bob.
	var blue []CompatibilityUser
	err := db.Find(ctx, "Team", "team-blue", &blue)
	require.NoError(t, err)
	require.Len(t, blue, 2)
	// Verify order: Alice (ID 1) before Bob (ID 2) per bbolt key order.
	assert.Equal(t, uint64(1), blue[0].ID)
	assert.Equal(t, "alice@example.test", blue[0].Email)
	assert.Equal(t, uint64(2), blue[1].ID)
	assert.Equal(t, "bob@example.test", blue[1].Email)

	// Team == team-red returns exactly Carol.
	var red []CompatibilityUser
	err = db.Find(ctx, "Team", "team-red", &red)
	require.NoError(t, err)
	require.Len(t, red, 1)
	assert.Equal(t, uint64(3), red[0].ID)
	assert.Equal(t, "carol@example.test", red[0].Email)

	// Missing indexed value returns ErrNotFound (verified against listSink.flush contract).
	var missing []CompatibilityUser
	err = db.Find(ctx, "Team", "team-void", &missing)
	require.ErrorIs(t, err, ErrNotFound)
	require.Empty(t, missing)
}

// ------------- E. Unique index compatibility -------------

func TestCompatibility_UniqueIndex(t *testing.T) {
	path := copyFixture(t)
	db := openFixture(t, path)
	defer assertClose(t, db)

	ctx := context.Background()

	// Every v5 Email unique index resolves to the correct record.
	var alice CompatibilityUser
	err := db.One(ctx, "Email", "alice@example.test", &alice)
	require.NoError(t, err)
	assert.Equal(t, uint64(1), alice.ID)

	var bob CompatibilityUser
	err = db.One(ctx, "Email", "bob@example.test", &bob)
	require.NoError(t, err)
	assert.Equal(t, uint64(2), bob.ID)

	var carol CompatibilityUser
	err = db.One(ctx, "Email", "carol@example.test", &carol)
	require.NoError(t, err)
	assert.Equal(t, uint64(3), carol.ID)

	var dave CompatibilityUser
	err = db.One(ctx, "Email", "dave@example.test", &dave)
	require.NoError(t, err)
	assert.Equal(t, uint64(4), dave.ID)

	// Saving a v6 record with a duplicate v5 Email must fail with ErrAlreadyExists.
	dup := CompatibilityUser{
		Email:    "alice@example.test",
		Team:     "team-dup",
		Name:     "Duplicate Email",
		Revision: 99,
	}
	err = db.Save(ctx, &dup)
	require.ErrorIs(t, err, ErrAlreadyExists)

	// The failed duplicate must not create a record, and ID must not have advanced.
	var all []CompatibilityUser
	err = db.All(ctx, &all)
	require.NoError(t, err)
	require.Len(t, all, 4, "duplicate save must not create extra records")

	// Existing unique index entries remain valid.
	err = db.One(ctx, "Email", "alice@example.test", &alice)
	require.NoError(t, err)
	assert.Equal(t, uint64(1), alice.ID)
	assert.Equal(t, "Alice", alice.Name)
}

// ------------- F. V6 update over v5 data -------------

func TestCompatibility_UpdateV5Data(t *testing.T) {
	path := copyFixture(t)
	db := openFixture(t, path)
	defer assertClose(t, db)

	ctx := context.Background()

	// Load Alice.
	var alice CompatibilityUser
	err := db.One(ctx, "Email", "alice@example.test", &alice)
	require.NoError(t, err)

	// Change Team, Name, and Revision.
	alice.Team = "team-red"
	alice.Name = "Alice Updated"
	alice.Revision = 99
	err = db.Update(ctx, &alice)
	require.NoError(t, err)

	// Verify the record changed.
	var updated CompatibilityUser
	err = db.One(ctx, "ID", alice.ID, &updated)
	require.NoError(t, err)
	assert.Equal(t, "team-red", updated.Team)
	assert.Equal(t, "Alice Updated", updated.Name)
	assert.Equal(t, 99, updated.Revision)

	// Old Team index no longer returns Alice.
	var oldTeam []CompatibilityUser
	err = db.Find(ctx, "Team", "team-blue", &oldTeam)
	require.NoError(t, err)
	for _, u := range oldTeam {
		assert.NotEqual(t, uint64(1), u.ID, "Alice should not be in team-blue after update")
	}
	assert.Len(t, oldTeam, 1)
	assert.Equal(t, uint64(2), oldTeam[0].ID) // only Bob remains

	// New Team index returns Alice (now in team-red).
	var newTeam []CompatibilityUser
	err = db.Find(ctx, "Team", "team-red", &newTeam)
	require.NoError(t, err)
	found := false
	for _, u := range newTeam {
		if u.ID == uint64(1) {
			found = true
			assert.Equal(t, "Alice Updated", u.Name)
		}
	}
	assert.True(t, found, "Alice should be in team-red")

	// Email unique index still resolves correctly.
	err = db.One(ctx, "Email", "alice@example.test", &alice)
	require.NoError(t, err)
	assert.Equal(t, "team-red", alice.Team)

	// Unrelated records remain unchanged.
	var bob CompatibilityUser
	err = db.One(ctx, "ID", uint64(2), &bob)
	require.NoError(t, err)
	assert.Equal(t, "bob@example.test", bob.Email)
	assert.Equal(t, "team-blue", bob.Team)
}

// ------------- G. V6 delete over v5 data -------------

func TestCompatibility_DeleteV5Data(t *testing.T) {
	path := copyFixture(t)
	db := openFixture(t, path)
	defer assertClose(t, db)

	ctx := context.Background()

	// Load Bob to get the struct with correct ID for deletion.
	var bob CompatibilityUser
	err := db.One(ctx, "Email", "bob@example.test", &bob)
	require.NoError(t, err)

	// Delete Bob using v6.
	err = db.DeleteStruct(ctx, &bob)
	require.NoError(t, err)

	// Direct lookup returns ErrNotFound.
	err = db.One(ctx, "ID", bob.ID, &CompatibilityUser{})
	require.ErrorIs(t, err, ErrNotFound)

	// Ordinary Team index no longer contains Bob.
	var blue []CompatibilityUser
	err = db.Find(ctx, "Team", "team-blue", &blue)
	require.NoError(t, err)
	assert.Len(t, blue, 1)
	assert.Equal(t, uint64(1), blue[0].ID) // only Alice

	// Bob's unique Email can be reused by a new v6 record.
	newUser := CompatibilityUser{
		Email:    "bob@example.test",
		Team:     "team-new",
		Name:     "New Bob",
		Revision: 10,
	}
	err = db.Save(ctx, &newUser)
	require.NoError(t, err)
	assert.Equal(t, uint64(5), newUser.ID)

	// Verify the new record can be looked up.
	err = db.One(ctx, "Email", "bob@example.test", &bob)
	require.NoError(t, err)
	assert.Equal(t, "New Bob", bob.Name)
	assert.Equal(t, "team-new", bob.Team)

	// Other v5 records remain readable.
	var alice CompatibilityUser
	err = db.One(ctx, "ID", uint64(1), &alice)
	require.NoError(t, err)
	assert.Equal(t, "alice@example.test", alice.Email)

	var carol CompatibilityUser
	err = db.One(ctx, "ID", uint64(3), &carol)
	require.NoError(t, err)
	assert.Equal(t, "carol@example.test", carol.Email)
}

// ------------- H. Nested bucket compatibility -------------

func TestCompatibility_NestedBucket(t *testing.T) {
	path := copyFixture(t)
	db := openFixture(t, path)
	defer assertClose(t, db)

	ctx := context.Background()

	// Access nested node.
	nested := db.From("tenant", "acme")

	// Read the nested v5 record.
	var nestedUsers []CompatibilityUser
	err := nested.All(ctx, &nestedUsers)
	require.NoError(t, err)
	require.Len(t, nestedUsers, 1)
	assert.Equal(t, "nested@example.test", nestedUsers[0].Email)
	assert.Equal(t, "team-nested", nestedUsers[0].Team)
	assert.Equal(t, "Nested User", nestedUsers[0].Name)
	assert.Equal(t, 5, nestedUsers[0].Revision)
	assert.NotZero(t, nestedUsers[0].ID)

	// Root queries do not include the nested record.
	var rootUsers []CompatibilityUser
	err = db.All(ctx, &rootUsers)
	require.NoError(t, err)
	for _, u := range rootUsers {
		assert.NotEqual(t, "nested@example.test", u.Email,
			"root query must not include nested record")
	}

	// Nested queries do not include root records.
	for _, u := range nestedUsers {
		assert.NotEqual(t, "alice@example.test", u.Email,
			"nested query must not include root records")
	}

	// Update the nested record with v6.
	nestedUsers[0].Name = "Nested Updated"
	nestedUsers[0].Revision = 42
	err = nested.Update(ctx, &nestedUsers[0])
	require.NoError(t, err)

	// Verify it persists in the nested path.
	var updated []CompatibilityUser
	err = nested.All(ctx, &updated)
	require.NoError(t, err)
	require.Len(t, updated, 1)
	assert.Equal(t, "Nested Updated", updated[0].Name)
	assert.Equal(t, 42, updated[0].Revision)

	// Root records remain unchanged.
	var alice CompatibilityUser
	err = db.One(ctx, "ID", uint64(1), &alice)
	require.NoError(t, err)
	assert.Equal(t, "Alice", alice.Name)
}

// ------------- I. KV compatibility -------------

func TestCompatibility_KV(t *testing.T) {
	path := copyFixture(t)
	db := openFixture(t, path)
	defer assertClose(t, db)

	ctx := context.Background()

	// Both fixed root KV values are readable with correct decoded types/values.
	var theme string
	err := db.Get(ctx, "settings", "theme", &theme)
	require.NoError(t, err)
	assert.Equal(t, "dark", theme)

	var retries int
	err = db.Get(ctx, "settings", "retries", &retries)
	require.NoError(t, err)
	assert.Equal(t, 3, retries)

	// Nested KV is readable only from the nested node.
	nested := db.From("tenant", "acme")
	var region string
	err = nested.Get(ctx, "settings", "region", &region)
	require.NoError(t, err)
	assert.Equal(t, "us-test-1", region)

	// Root cannot read nested KV.
	err = db.Get(ctx, "settings", "region", &region)
	require.ErrorIs(t, err, ErrNotFound)

	// Update a v5 KV value with v6.
	err = db.Set(ctx, "settings", "theme", "light")
	require.NoError(t, err)

	var updatedTheme string
	err = db.Get(ctx, "settings", "theme", &updatedTheme)
	require.NoError(t, err)
	assert.Equal(t, "light", updatedTheme)

	// Add a new KV value with v6.
	err = db.Set(ctx, "settings", "enabled", true)
	require.NoError(t, err)

	var enabled bool
	err = db.Get(ctx, "settings", "enabled", &enabled)
	require.NoError(t, err)
	assert.True(t, enabled)

	// Existing values remain intact.
	err = db.Get(ctx, "settings", "retries", &retries)
	require.NoError(t, err)
	assert.Equal(t, 3, retries)
}

// ------------- J. Metadata/version and reopen -------------

func TestCompatibility_MetadataReopen(t *testing.T) {
	path := copyFixture(t)
	ctx := context.Background()

	var newUser CompatibilityUser
	func() {
		db := openFixture(t, path)
		defer assertClose(t, db)

		// v6 opens the untouched v5 fixture without version rejection.
		// Default codec reads succeed (proven by reading records).
		var u1 CompatibilityUser
		err := db.One(ctx, "ID", uint64(1), &u1)
		require.NoError(t, err)
		assert.Equal(t, "alice@example.test", u1.Email)

		// Write a new record with v6.
		newUser = CompatibilityUser{
			Email:    "v6write@example.test",
			Team:     "team-v6",
			Name:     "V6 Writer",
			Revision: 100,
		}
		err = db.Save(ctx, &newUser)
		require.NoError(t, err)
		assert.Equal(t, uint64(5), newUser.ID)
	}()

	// Reopen and verify all expected old/new data remains readable.
	db2, err := Open(context.Background(), path)
	require.NoError(t, err, "reopen after v6 writes")
	defer assertClose(t, db2)

	// v5 data still readable.
	var u1 CompatibilityUser
	err = db2.One(ctx, "ID", uint64(1), &u1)
	require.NoError(t, err)
	assert.Equal(t, "alice@example.test", u1.Email)

	err = db2.One(ctx, "ID", uint64(4), &u1)
	require.NoError(t, err)
	assert.Equal(t, "dave@example.test", u1.Email)

	// v6 data readable.
	err = db2.One(ctx, "ID", newUser.ID, &u1)
	require.NoError(t, err)
	assert.Equal(t, "v6write@example.test", u1.Email)

	// Indexes and unique constraints still work.
	var v6Team []CompatibilityUser
	err = db2.Find(ctx, "Team", "team-v6", &v6Team)
	require.NoError(t, err)
	require.Len(t, v6Team, 1)
	assert.Equal(t, "v6write@example.test", v6Team[0].Email)

	err = db2.One(ctx, "Email", "v6write@example.test", &u1)
	require.NoError(t, err)
	assert.Equal(t, "V6 Writer", u1.Name)

	// KV data retained.
	var theme string
	err = db2.Get(ctx, "settings", "theme", &theme)
	require.NoError(t, err)
	assert.Equal(t, "dark", theme)
}
