package rainstorm

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
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

// CompatibilityNamedEvent matches the v5.3.0 BucketNamer type used in the generator.
type CompatibilityNamedEvent struct {
	ID       uint64 `rainstorm:"id,increment"`
	Code     string `rainstorm:"unique"`
	Category string `rainstorm:"index"`
	Message  string
}

func (CompatibilityNamedEvent) RainstormBucketName() string {
	return "compatibility_named_events"
}

// ------------- manifest types for test A -------------

type compatManifest struct {
	Source          compatSource          `json:"source"`
	Codec           string                `json:"codec"`
	RootRecords     []compatRootRec       `json:"root_records"`
	Indexes         compatIndexes         `json:"indexes"`
	Nested          compatNested          `json:"nested"`
	KV              compatKVSection       `json:"kv"`
	BucketNamer     compatBucketNamer     `json:"bucket_namer"`
	RuntimeExplicit compatRuntimeExplicit `json:"runtime_explicit"`
	RuntimeRootData compatRuntimeRootData `json:"runtime_root_data"`
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

// R6.5B1 manifest types.

type compatBucketNamer struct {
	GoTypeName       string               `json:"go_type_name"`
	CustomBucketName string               `json:"custom_bucket_name"`
	ReceiverForm     string               `json:"receiver_form"`
	Records          []compatNamedRec     `json:"records"`
	Indexes          compatBucketNamerIdx `json:"indexes"`
}

type compatNamedRec struct {
	ID       uint64 `json:"id"`
	Code     string `json:"code"`
	Category string `json:"category"`
	Message  string `json:"message"`
}

type compatBucketNamerIdx struct {
	Category compatListIdx   `json:"category"`
	Code     compatUniqueIdx `json:"code"`
}

type compatRuntimeExplicit struct {
	Path           []string               `json:"path"`
	GoType         string                 `json:"go_type"`
	BucketBehavior string                 `json:"bucket_behavior"`
	FieldSchema    map[string]compatField `json:"field_schema"`
	Records        []compatRuntimeRec     `json:"records"`
	Indexes        compatRuntimeIdx       `json:"indexes"`
}

type compatRuntimeRootData struct {
	Path           []string               `json:"path"`
	GoType         string                 `json:"go_type"`
	BucketBehavior string                 `json:"bucket_behavior"`
	FieldSchema    map[string]compatField `json:"field_schema"`
	Records        []compatRuntimeRec     `json:"records"`
	Indexes        compatRuntimeIdx       `json:"indexes"`
}

type compatField struct {
	GoType string `json:"go_type"`
	Tag    string `json:"tag"`
}

type compatRuntimeIdx struct {
	Group compatListIdx   `json:"group"`
	Slug  compatUniqueIdx `json:"slug"`
}

type compatRuntimeRec struct {
	ID       uint64 `json:"id"`
	Slug     string `json:"slug"`
	Group    string `json:"group"`
	Label    string `json:"label"`
	Revision int    `json:"revision"`
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

// makeRuntimeExplicitType returns the reflect.Type matching the runtime/explicit
// schema used by the generator.
func makeRuntimeExplicitType() reflect.Type {
	return reflect.StructOf([]reflect.StructField{
		{
			Name: "ID",
			Type: reflect.TypeOf(uint64(0)),
			Tag:  reflect.StructTag(`rainstorm:"id,increment"`),
		},
		{
			Name: "Slug",
			Type: reflect.TypeOf(""),
			Tag:  reflect.StructTag(`rainstorm:"unique"`),
		},
		{
			Name: "Group",
			Type: reflect.TypeOf(""),
			Tag:  reflect.StructTag(`rainstorm:"index"`),
		},
		{
			Name: "Label",
			Type: reflect.TypeOf(""),
		},
		{
			Name: "Revision",
			Type: reflect.TypeOf(0),
		},
	})
}

// makeRuntimeRootDataType returns the reflect.Type matching the runtime/root-data
// schema used by the generator (structurally identical to explicit).
func makeRuntimeRootDataType() reflect.Type {
	return reflect.StructOf([]reflect.StructField{
		{
			Name: "ID",
			Type: reflect.TypeOf(uint64(0)),
			Tag:  reflect.StructTag(`rainstorm:"id,increment"`),
		},
		{
			Name: "Slug",
			Type: reflect.TypeOf(""),
			Tag:  reflect.StructTag(`rainstorm:"unique"`),
		},
		{
			Name: "Group",
			Type: reflect.TypeOf(""),
			Tag:  reflect.StructTag(`rainstorm:"index"`),
		},
		{
			Name: "Label",
			Type: reflect.TypeOf(""),
		},
		{
			Name: "Revision",
			Type: reflect.TypeOf(0),
		},
	})
}

// runtimeRecFromValue extracts field values from a reflect.Value (pointer to runtime struct).
func runtimeRecFromValue(v reflect.Value) compatRuntimeRec {
	e := v.Elem()
	return compatRuntimeRec{
		ID:       e.FieldByName("ID").Uint(),
		Slug:     e.FieldByName("Slug").String(),
		Group:    e.FieldByName("Group").String(),
		Label:    e.FieldByName("Label").String(),
		Revision: int(e.FieldByName("Revision").Int()),
	}
}

// runtimeSliceFromPtr extracts the slice from a pointer-to-slice reflect.Value.
// Returns the slice value and its length.
func runtimeSliceFromPtr(ptr reflect.Value) (reflect.Value, int) {
	sl := ptr.Elem()
	return sl, sl.Len()
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

	// R6.5B1: bucket_namer section.
	assert.Equal(t, "CompatibilityNamedEvent", m.BucketNamer.GoTypeName)
	assert.Equal(t, "compatibility_named_events", m.BucketNamer.CustomBucketName)
	assert.Len(t, m.BucketNamer.Records, 3)
	assert.Equal(t, uint64(1), m.BucketNamer.Records[0].ID)
	assert.Equal(t, "event-alpha", m.BucketNamer.Records[0].Code)
	assert.Equal(t, uint64(2), m.BucketNamer.Records[1].ID)
	assert.Equal(t, "event-beta", m.BucketNamer.Records[1].Code)
	assert.Equal(t, uint64(3), m.BucketNamer.Records[2].ID)
	assert.Equal(t, "event-gamma", m.BucketNamer.Records[2].Code)

	// R6.5B1: runtime_explicit section.
	assert.Equal(t, []string{"runtime", "explicit"}, m.RuntimeExplicit.Path)
	assert.Len(t, m.RuntimeExplicit.Records, 3)
	assert.Equal(t, uint64(1), m.RuntimeExplicit.Records[0].ID)
	assert.Equal(t, "runtime-alpha", m.RuntimeExplicit.Records[0].Slug)
	assert.Equal(t, uint64(2), m.RuntimeExplicit.Records[1].ID)
	assert.Equal(t, "runtime-beta", m.RuntimeExplicit.Records[1].Slug)
	assert.Equal(t, uint64(3), m.RuntimeExplicit.Records[2].ID)
	assert.Equal(t, "runtime-gamma", m.RuntimeExplicit.Records[2].Slug)

	// R6.5B1: runtime_root_data section.
	assert.Equal(t, []string{"runtime", "root-data"}, m.RuntimeRootData.Path)
	assert.Len(t, m.RuntimeRootData.Records, 2)
	assert.Equal(t, uint64(1), m.RuntimeRootData.Records[0].ID)
	assert.Equal(t, "root-runtime-alpha", m.RuntimeRootData.Records[0].Slug)
	assert.Equal(t, uint64(2), m.RuntimeRootData.Records[1].ID)
	assert.Equal(t, "root-runtime-beta", m.RuntimeRootData.Records[1].Slug)
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

// ======================================================================
// R6.5B1: BucketNamer, runtime struct, and extended fixture tests
// ======================================================================

// ------------- K. BucketNamer read -------------

func TestCompatibility_BucketNamerRead(t *testing.T) {
	path := copyFixture(t)
	db := openFixture(t, path)
	defer assertClose(t, db)

	ctx := context.Background()

	// Read all three named events via the custom bucket name.
	// The v6 type CompatibilityNamedEvent implements RainstormBucketName
	// which returns "compatibility_named_events", matching v5's custom bucket.
	var all []CompatibilityNamedEvent
	err := db.All(ctx, &all)
	require.NoError(t, err)
	require.Len(t, all, 3)

	byCode := make(map[string]CompatibilityNamedEvent, 3)
	for _, e := range all {
		byCode[e.Code] = e
	}

	// Verify every record matches the manifest.
	require.Contains(t, byCode, "event-alpha")
	require.Contains(t, byCode, "event-beta")
	require.Contains(t, byCode, "event-gamma")
	alpha := byCode["event-alpha"]
	assert.Equal(t, uint64(1), alpha.ID)
	assert.Equal(t, "audit", alpha.Category)
	assert.Equal(t, "Alpha", alpha.Message)

	beta := byCode["event-beta"]
	assert.Equal(t, uint64(2), beta.ID)
	assert.Equal(t, "audit", beta.Category)
	assert.Equal(t, "Beta", beta.Message)

	gamma := byCode["event-gamma"]
	assert.Equal(t, uint64(3), gamma.ID)
	assert.Equal(t, "system", gamma.Category)
	assert.Equal(t, "Gamma", gamma.Message)

	// Category == audit returns exactly Alpha then Beta in order.
	var audit []CompatibilityNamedEvent
	err = db.Find(ctx, "Category", "audit", &audit)
	require.NoError(t, err)
	require.Len(t, audit, 2)
	assert.Equal(t, uint64(1), audit[0].ID)
	assert.Equal(t, "event-alpha", audit[0].Code)
	assert.Equal(t, uint64(2), audit[1].ID)
	assert.Equal(t, "event-beta", audit[1].Code)

	// Code unique lookups resolve every record.
	var ev1 CompatibilityNamedEvent
	err = db.One(ctx, "Code", "event-alpha", &ev1)
	require.NoError(t, err)
	assert.Equal(t, uint64(1), ev1.ID)
	assert.Equal(t, "audit", ev1.Category)

	var ev2 CompatibilityNamedEvent
	err = db.One(ctx, "Code", "event-beta", &ev2)
	require.NoError(t, err)
	assert.Equal(t, uint64(2), ev2.ID)

	var ev3 CompatibilityNamedEvent
	err = db.One(ctx, "Code", "event-gamma", &ev3)
	require.NoError(t, err)
	assert.Equal(t, uint64(3), ev3.ID)
	assert.Equal(t, "system", ev3.Category)

	// Prove the default Go type-name bucket does NOT contain the data.
	// If bucket naming didn't work, the type-name "CompatibilityNamedEvent" bucket
	// would have been used, not "compatibility_named_events".
	node := db.From("CompatibilityNamedEvent")
	var wrongBucket []CompatibilityNamedEvent
	err = node.All(ctx, &wrongBucket)
	require.NoError(t, err)
	require.Len(t, wrongBucket, 0, "Go type-name bucket must not contain named events")

	// Root CompatibilityUser queries remain in their own bucket.
	var users []CompatibilityUser
	err = db.All(ctx, &users)
	require.NoError(t, err)
	require.Len(t, users, 4)
}

// ------------- L. BucketNamer v6 mutation -------------

func TestCompatibility_BucketNamerV6Mutation(t *testing.T) {
	path := copyFixture(t)
	db := openFixture(t, path)
	defer assertClose(t, db)

	ctx := context.Background()

	// Load event-alpha.
	var alpha CompatibilityNamedEvent
	err := db.One(ctx, "Code", "event-alpha", &alpha)
	require.NoError(t, err)
	assert.Equal(t, uint64(1), alpha.ID)
	assert.Equal(t, "audit", alpha.Category)

	// Change Category from audit to system and change Message.
	alpha.Category = "system"
	alpha.Message = "Alpha Modified"
	err = db.Update(ctx, &alpha)
	require.NoError(t, err)

	// Prove old Category (audit) no longer returns alpha.
	var audit []CompatibilityNamedEvent
	err = db.Find(ctx, "Category", "audit", &audit)
	require.NoError(t, err)
	require.Len(t, audit, 1)
	assert.Equal(t, uint64(2), audit[0].ID)
	assert.Equal(t, "event-beta", audit[0].Code)

	// Prove new Category (system) now contains alpha.
	var system []CompatibilityNamedEvent
	err = db.Find(ctx, "Category", "system", &system)
	require.NoError(t, err)
	foundAlpha := false
	for _, e := range system {
		if e.ID == uint64(1) {
			foundAlpha = true
			assert.Equal(t, "Alpha Modified", e.Message)
		}
	}
	assert.True(t, foundAlpha, "alpha should be in system after update")
	assert.Equal(t, 2, len(system)) // alpha and gamma

	// Code unique lookup remains correct after update.
	var updatedAlpha CompatibilityNamedEvent
	err = db.One(ctx, "Code", "event-alpha", &updatedAlpha)
	require.NoError(t, err)
	assert.Equal(t, uint64(1), updatedAlpha.ID)
	assert.Equal(t, "system", updatedAlpha.Category)
	assert.Equal(t, "Alpha Modified", updatedAlpha.Message)

	// Delete event-beta.
	var beta CompatibilityNamedEvent
	err = db.One(ctx, "Code", "event-beta", &beta)
	require.NoError(t, err)
	assert.Equal(t, uint64(2), beta.ID)

	err = db.DeleteStruct(ctx, &beta)
	require.NoError(t, err)

	// Direct lookup fails with ErrNotFound.
	err = db.One(ctx, "Code", "event-beta", &beta)
	require.ErrorIs(t, err, ErrNotFound)

	// Audit index no longer contains beta.
	var auditAfter []CompatibilityNamedEvent
	err = db.Find(ctx, "Category", "audit", &auditAfter)
	require.ErrorIs(t, err, ErrNotFound)
	require.Len(t, auditAfter, 0)

	// Save a new record reusing Code event-beta.
	newEv := CompatibilityNamedEvent{
		Code:     "event-beta",
		Category: "system",
		Message:  "Reused Beta",
	}
	err = db.Save(ctx, &newEv)
	require.NoError(t, err)
	assert.Equal(t, uint64(4), newEv.ID, "next incremented ID after max (3)")

	// Prove new record is readable via Code.
	var newBeta CompatibilityNamedEvent
	err = db.One(ctx, "Code", "event-beta", &newBeta)
	require.NoError(t, err)
	assert.Equal(t, uint64(4), newBeta.ID)
	assert.Equal(t, "system", newBeta.Category)
	assert.Equal(t, "Reused Beta", newBeta.Message)

	// Unrelated event-gamma remains unchanged.
	var gamma CompatibilityNamedEvent
	err = db.One(ctx, "Code", "event-gamma", &gamma)
	require.NoError(t, err)
	assert.Equal(t, uint64(3), gamma.ID)
	assert.Equal(t, "system", gamma.Category)
	assert.Equal(t, "Gamma", gamma.Message)
}

// ------------- M. Runtime explicit read -------------

func TestCompatibility_RuntimeExplicitRead(t *testing.T) {
	path := copyFixture(t)
	db := openFixture(t, path)
	defer assertClose(t, db)

	ctx := context.Background()
	m := readManifest(t)

	runtimeType := makeRuntimeExplicitType()
	node := db.From("runtime", "explicit")

	// Read all three records via All.
	sliceType := reflect.SliceOf(reflect.PointerTo(runtimeType))
	resultsPtr := reflect.New(sliceType)
	err := node.All(ctx, resultsPtr.Interface())
	require.NoError(t, err)

	sl, n := runtimeSliceFromPtr(resultsPtr)
	require.Equal(t, 3, n)

	// Build lookup by Slug.
	bySlug := make(map[string]compatRuntimeRec, 3)
	for i := 0; i < n; i++ {
		r := runtimeRecFromValue(sl.Index(i))
		bySlug[r.Slug] = r
	}

	// Verify every record matches manifest.
	require.Contains(t, bySlug, "runtime-alpha")
	require.Contains(t, bySlug, "runtime-beta")
	require.Contains(t, bySlug, "runtime-gamma")
	alpha := bySlug["runtime-alpha"]
	assert.Equal(t, uint64(1), alpha.ID)
	assert.Equal(t, "group-shared", alpha.Group)
	assert.Equal(t, "Runtime Alpha", alpha.Label)
	assert.Equal(t, 1, alpha.Revision)

	beta := bySlug["runtime-beta"]
	assert.Equal(t, uint64(2), beta.ID)
	assert.Equal(t, "group-shared", beta.Group)
	assert.Equal(t, "Runtime Beta", beta.Label)
	assert.Equal(t, 1, beta.Revision)

	gamma := bySlug["runtime-gamma"]
	assert.Equal(t, uint64(3), gamma.ID)
	assert.Equal(t, "group-other", gamma.Group)
	assert.Equal(t, "Runtime Gamma", gamma.Label)
	assert.Equal(t, 2, gamma.Revision)

	// Group == group-shared returns exactly alpha and beta in order.
	findSliceType := reflect.SliceOf(reflect.PointerTo(runtimeType))
	findResults := reflect.New(findSliceType)
	err = node.Find(ctx, "Group", "group-shared", findResults.Interface())
	require.NoError(t, err)
	findSl, findN := runtimeSliceFromPtr(findResults)
	require.Equal(t, 2, findN)
	assert.Equal(t, uint64(1), findSl.Index(0).Elem().FieldByName("ID").Uint())
	assert.Equal(t, "runtime-alpha", findSl.Index(0).Elem().FieldByName("Slug").String())
	assert.Equal(t, uint64(2), findSl.Index(1).Elem().FieldByName("ID").Uint())
	assert.Equal(t, "runtime-beta", findSl.Index(1).Elem().FieldByName("Slug").String())

	// Every Slug unique lookup resolves correctly.
	for slug, expectedID := range m.RuntimeExplicit.Indexes.Slug.Lookups {
		oneResult := reflect.New(runtimeType).Interface()
		err = node.One(ctx, "Slug", slug, oneResult)
		require.NoError(t, err)
		assert.Equal(t, expectedID, reflect.ValueOf(oneResult).Elem().FieldByName("ID").Uint(),
			"slug %s should resolve to ID %d", slug, expectedID)
	}

	// Root CompatibilityUser queries do not include runtime records.
	var users []CompatibilityUser
	err = db.All(ctx, &users)
	require.NoError(t, err)
	require.Len(t, users, 4)

	// Anonymous runtime types require an explicit node because they have no
	// static bucket name. This is an API precondition, not an isolation proof.
	emptyResults := reflect.New(findSliceType)
	err = db.Find(ctx, "Slug", "runtime-alpha", emptyResults.Interface())
	require.ErrorIs(t, err, ErrNoName)
}

// ------------- N. Runtime explicit v6 mutation -------------

func TestCompatibility_RuntimeExplicitV6Mutation(t *testing.T) {
	path := copyFixture(t)
	db := openFixture(t, path)
	defer assertClose(t, db)

	ctx := context.Background()

	runtimeType := makeRuntimeExplicitType()
	node := db.From("runtime", "explicit")

	// Load runtime-alpha via its unique Slug.
	alphaPtr := reflect.New(runtimeType).Interface()
	err := node.One(ctx, "Slug", "runtime-alpha", alphaPtr)
	require.NoError(t, err)
	alphaVal := reflect.ValueOf(alphaPtr).Elem()
	assert.Equal(t, uint64(1), alphaVal.FieldByName("ID").Uint())
	assert.Equal(t, "group-shared", alphaVal.FieldByName("Group").String())

	// Mutate Group, Label, and Revision using reflection.
	alphaVal.FieldByName("Group").SetString("group-other")
	alphaVal.FieldByName("Label").SetString("Runtime Alpha Modified")
	alphaVal.FieldByName("Revision").SetInt(99)

	err = node.Update(ctx, alphaPtr)
	require.NoError(t, err)

	// Read it back and verify all changed fields.
	updatedPtr := reflect.New(runtimeType).Interface()
	err = node.One(ctx, "Slug", "runtime-alpha", updatedPtr)
	require.NoError(t, err)
	updatedVal := reflect.ValueOf(updatedPtr).Elem()
	assert.Equal(t, uint64(1), updatedVal.FieldByName("ID").Uint())
	assert.Equal(t, "group-other", updatedVal.FieldByName("Group").String())
	assert.Equal(t, "Runtime Alpha Modified", updatedVal.FieldByName("Label").String())
	assert.Equal(t, int64(99), updatedVal.FieldByName("Revision").Int())

	// Prove old Group index loses it.
	sliceType := reflect.SliceOf(reflect.PointerTo(runtimeType))
	oldGroupResults := reflect.New(sliceType)
	err = node.Find(ctx, "Group", "group-shared", oldGroupResults.Interface())
	require.NoError(t, err)
	oldSl, oldN := runtimeSliceFromPtr(oldGroupResults)
	require.Equal(t, 1, oldN)
	assert.Equal(t, uint64(2), oldSl.Index(0).Elem().FieldByName("ID").Uint()) // only beta
	assert.Equal(t, "runtime-beta", oldSl.Index(0).Elem().FieldByName("Slug").String())

	// Prove new Group index contains alpha and the pre-existing gamma.
	newGroupResults := reflect.New(sliceType)
	err = node.Find(ctx, "Group", "group-other", newGroupResults.Interface())
	require.NoError(t, err)
	newSl, newN := runtimeSliceFromPtr(newGroupResults)
	require.Equal(t, 2, newN)
	require.Equal(t, uint64(1), newSl.Index(0).Elem().FieldByName("ID").Uint())
	require.Equal(t, "runtime-alpha", newSl.Index(0).Elem().FieldByName("Slug").String())
	require.Equal(t, uint64(3), newSl.Index(1).Elem().FieldByName("ID").Uint())
	require.Equal(t, "runtime-gamma", newSl.Index(1).Elem().FieldByName("Slug").String())

	// Slug unique lookup remains correct after update.
	slugPtr := reflect.New(runtimeType).Interface()
	err = node.One(ctx, "Slug", "runtime-alpha", slugPtr)
	require.NoError(t, err)
	assert.Equal(t, "Runtime Alpha Modified", reflect.ValueOf(slugPtr).Elem().FieldByName("Label").String())

	// Delete runtime-beta.
	betaPtr := reflect.New(runtimeType).Interface()
	err = node.One(ctx, "Slug", "runtime-beta", betaPtr)
	require.NoError(t, err)
	betaVal := reflect.ValueOf(betaPtr).Elem()
	assert.Equal(t, uint64(2), betaVal.FieldByName("ID").Uint())

	err = node.DeleteStruct(ctx, betaPtr)
	require.NoError(t, err)

	// Prove direct lookup fails with ErrNotFound.
	err = node.One(ctx, "Slug", "runtime-beta", reflect.New(runtimeType).Interface())
	require.ErrorIs(t, err, ErrNotFound)

	// Group index cleanup: group-shared now empty.
	err = node.Find(ctx, "Group", "group-shared", oldGroupResults.Interface())
	require.ErrorIs(t, err, ErrNotFound)

	// Save a new runtime value reusing runtime-beta's Slug.
	newPtr := reflect.New(runtimeType)
	newVal := newPtr.Elem()
	newVal.FieldByName("Slug").SetString("runtime-beta")
	newVal.FieldByName("Group").SetString("group-new")
	newVal.FieldByName("Label").SetString("Runtime Beta New")
	newVal.FieldByName("Revision").SetInt(10)
	err = node.Save(ctx, newPtr.Interface())
	require.NoError(t, err)
	assert.Equal(t, uint64(4), newVal.FieldByName("ID").Uint(), "next ID")

	// Prove new record readable.
	reusedPtr := reflect.New(runtimeType).Interface()
	err = node.One(ctx, "Slug", "runtime-beta", reusedPtr)
	require.NoError(t, err)
	assert.Equal(t, uint64(4), reflect.ValueOf(reusedPtr).Elem().FieldByName("ID").Uint())
	assert.Equal(t, "Runtime Beta New", reflect.ValueOf(reusedPtr).Elem().FieldByName("Label").String())

	// runtime-gamma remains unchanged.
	gammaPtr := reflect.New(runtimeType).Interface()
	err = node.One(ctx, "Slug", "runtime-gamma", gammaPtr)
	require.NoError(t, err)
	gammaVal := reflect.ValueOf(gammaPtr).Elem()
	assert.Equal(t, uint64(3), gammaVal.FieldByName("ID").Uint())
	assert.Equal(t, "group-other", gammaVal.FieldByName("Group").String())
	assert.Equal(t, "Runtime Gamma", gammaVal.FieldByName("Label").String())
}

// ------------- O. Runtime root-data read -------------

func TestCompatibility_RuntimeRootDataRead(t *testing.T) {
	path := copyFixture(t)
	db := openFixture(t, path)
	defer assertClose(t, db)

	ctx := context.Background()
	m := readManifest(t)

	runtimeType := makeRuntimeRootDataType()
	node := db.From("runtime", "root-data")

	// Read all records via All. The node root itself is the data bucket.
	sliceType := reflect.SliceOf(reflect.PointerTo(runtimeType))
	resultsPtr := reflect.New(sliceType)
	err := node.All(ctx, resultsPtr.Interface())
	require.NoError(t, err)

	sl, n := runtimeSliceFromPtr(resultsPtr)
	require.Equal(t, 2, n)

	bySlug := make(map[string]compatRuntimeRec, 2)
	for i := 0; i < n; i++ {
		r := runtimeRecFromValue(sl.Index(i))
		bySlug[r.Slug] = r
	}

	// Verify exactly two records match manifest.
	require.Contains(t, bySlug, "root-runtime-alpha")
	require.Contains(t, bySlug, "root-runtime-beta")
	alpha := bySlug["root-runtime-alpha"]
	assert.Equal(t, uint64(1), alpha.ID)
	assert.Equal(t, "root-group", alpha.Group)
	assert.Equal(t, "Root Runtime Alpha", alpha.Label)
	assert.Equal(t, 3, alpha.Revision)

	beta := bySlug["root-runtime-beta"]
	assert.Equal(t, uint64(2), beta.ID)
	assert.Equal(t, "root-group", beta.Group)
	assert.Equal(t, "Root Runtime Beta", beta.Label)
	assert.Equal(t, 4, beta.Revision)

	// Group index works: root-group returns both in order.
	findSliceType := reflect.SliceOf(reflect.PointerTo(runtimeType))
	findResults := reflect.New(findSliceType)
	err = node.Find(ctx, "Group", "root-group", findResults.Interface())
	require.NoError(t, err)
	findSl, findN := runtimeSliceFromPtr(findResults)
	require.Equal(t, 2, findN)
	assert.Equal(t, uint64(1), findSl.Index(0).Elem().FieldByName("ID").Uint())
	assert.Equal(t, "root-runtime-alpha", findSl.Index(0).Elem().FieldByName("Slug").String())
	assert.Equal(t, uint64(2), findSl.Index(1).Elem().FieldByName("ID").Uint())
	assert.Equal(t, "root-runtime-beta", findSl.Index(1).Elem().FieldByName("Slug").String())

	// Slug unique lookups work.
	for slug, expectedID := range m.RuntimeRootData.Indexes.Slug.Lookups {
		oneResult := reflect.New(runtimeType).Interface()
		err = node.One(ctx, "Slug", slug, oneResult)
		require.NoError(t, err)
		assert.Equal(t, expectedID, reflect.ValueOf(oneResult).Elem().FieldByName("ID").Uint(),
			"slug %s should resolve to ID %d", slug, expectedID)
	}

	// The separate runtime/explicit path does not contain root-data records.
	explicitNode := db.From("runtime", "explicit")
	otherResults := reflect.New(sliceType)
	err = explicitNode.Find(ctx, "Slug", "root-runtime-alpha", otherResults.Interface())
	require.ErrorIs(t, err, ErrNotFound)
	require.Zero(t, otherResults.Elem().Len())

	// Existing root named-type datasets remain intact in their own buckets.
	var users []CompatibilityUser
	err = db.All(ctx, &users)
	require.NoError(t, err)
	require.Len(t, users, 4)
	var events []CompatibilityNamedEvent
	err = db.All(ctx, &events)
	require.NoError(t, err)
	require.Len(t, events, 3)
}

// ------------- P. Runtime root-data v6 mutation -------------

func TestCompatibility_RuntimeRootDataV6Mutation(t *testing.T) {
	path := copyFixture(t)
	ctx := context.Background()

	runtimeType := makeRuntimeRootDataType()
	nodePath := []string{"runtime", "root-data"}

	// First session: mutate and verify.
	var nextID uint64
	func() {
		db := openFixture(t, path)
		defer assertClose(t, db)

		node := db.From(nodePath...)

		// Load root-runtime-alpha.
		alphaPtr := reflect.New(runtimeType).Interface()
		err := node.One(ctx, "Slug", "root-runtime-alpha", alphaPtr)
		require.NoError(t, err)
		alphaVal := reflect.ValueOf(alphaPtr).Elem()
		assert.Equal(t, uint64(1), alphaVal.FieldByName("ID").Uint())

		// Update: change Group, Label, Revision.
		alphaVal.FieldByName("Group").SetString("root-group-modified")
		alphaVal.FieldByName("Label").SetString("Root Alpha Updated")
		alphaVal.FieldByName("Revision").SetInt(42)
		err = node.Update(ctx, alphaPtr)
		require.NoError(t, err)

		// Read back and verify.
		updatedPtr := reflect.New(runtimeType).Interface()
		err = node.One(ctx, "Slug", "root-runtime-alpha", updatedPtr)
		require.NoError(t, err)
		updatedVal := reflect.ValueOf(updatedPtr).Elem()
		assert.Equal(t, uint64(1), updatedVal.FieldByName("ID").Uint())
		assert.Equal(t, "root-group-modified", updatedVal.FieldByName("Group").String())
		assert.Equal(t, "Root Alpha Updated", updatedVal.FieldByName("Label").String())
		assert.Equal(t, int64(42), updatedVal.FieldByName("Revision").Int())

		// Old Group index no longer contains alpha.
		sliceType := reflect.SliceOf(reflect.PointerTo(runtimeType))
		oldResults := reflect.New(sliceType)
		err = node.Find(ctx, "Group", "root-group", oldResults.Interface())
		require.NoError(t, err)
		oldSl, oldN := runtimeSliceFromPtr(oldResults)
		require.Equal(t, 1, oldN)
		assert.Equal(t, uint64(2), oldSl.Index(0).Elem().FieldByName("ID").Uint()) // only beta

		// New Group index contains alpha.
		newResults := reflect.New(sliceType)
		err = node.Find(ctx, "Group", "root-group-modified", newResults.Interface())
		require.NoError(t, err)
		newSl, newN := runtimeSliceFromPtr(newResults)
		require.Equal(t, 1, newN)
		assert.Equal(t, uint64(1), newSl.Index(0).Elem().FieldByName("ID").Uint())

		// Delete root-runtime-beta.
		betaPtr := reflect.New(runtimeType).Interface()
		err = node.One(ctx, "Slug", "root-runtime-beta", betaPtr)
		require.NoError(t, err)
		err = node.DeleteStruct(ctx, betaPtr)
		require.NoError(t, err)

		// Prove deleted: lookup fails with ErrNotFound.
		err = node.One(ctx, "Slug", "root-runtime-beta", reflect.New(runtimeType).Interface())
		require.ErrorIs(t, err, ErrNotFound)

		// root-group index now empty.
		err = node.Find(ctx, "Group", "root-group", oldResults.Interface())
		require.ErrorIs(t, err, ErrNotFound)

		// Save a replacement reusing the deleted unique Slug.
		newPtr := reflect.New(runtimeType)
		newVal := newPtr.Elem()
		newVal.FieldByName("Slug").SetString("root-runtime-beta")
		newVal.FieldByName("Group").SetString("root-group-replacement")
		newVal.FieldByName("Label").SetString("Replacement Beta")
		newVal.FieldByName("Revision").SetInt(99)
		err = node.Save(ctx, newPtr.Interface())
		require.NoError(t, err)
		nextID = newVal.FieldByName("ID").Uint()
		assert.Equal(t, uint64(3), nextID, "next incremented ID after max (2)")

		// Prove replacement readable via Slug.
		reusedPtr := reflect.New(runtimeType).Interface()
		err = node.One(ctx, "Slug", "root-runtime-beta", reusedPtr)
		require.NoError(t, err)
		assert.Equal(t, "Replacement Beta", reflect.ValueOf(reusedPtr).Elem().FieldByName("Label").String())
	}()

	// Reopen with v6 and prove all changes persist.
	db2, err := Open(context.Background(), path)
	require.NoError(t, err, "reopen after runtime root-data mutations")
	defer assertClose(t, db2)

	node2 := db2.From(nodePath...)

	// alpha update persisted.
	alphaPtr := reflect.New(runtimeType).Interface()
	err = node2.One(ctx, "Slug", "root-runtime-alpha", alphaPtr)
	require.NoError(t, err)
	alphaVal := reflect.ValueOf(alphaPtr).Elem()
	assert.Equal(t, uint64(1), alphaVal.FieldByName("ID").Uint())
	assert.Equal(t, "root-group-modified", alphaVal.FieldByName("Group").String())
	assert.Equal(t, "Root Alpha Updated", alphaVal.FieldByName("Label").String())
	assert.Equal(t, int64(42), alphaVal.FieldByName("Revision").Int())

	// beta delete and replacement persisted: Slug "root-runtime-beta"
	// now resolves to the replacement (ID 3).
	betaSlugPtr := reflect.New(runtimeType).Interface()
	err = node2.One(ctx, "Slug", "root-runtime-beta", betaSlugPtr)
	require.NoError(t, err)
	betaSlugVal := reflect.ValueOf(betaSlugPtr).Elem()
	assert.Equal(t, uint64(3), betaSlugVal.FieldByName("ID").Uint())
	assert.Equal(t, "Replacement Beta", betaSlugVal.FieldByName("Label").String())

	// Old beta ID (2) is gone.
	oldBetaPtr := reflect.New(runtimeType).Interface()
	err = node2.One(ctx, "ID", uint64(2), oldBetaPtr)
	require.ErrorIs(t, err, ErrNotFound)

	// replacement persisted via its ID.
	reusedPtr := reflect.New(runtimeType).Interface()
	err = node2.One(ctx, "ID", nextID, reusedPtr)
	require.NoError(t, err)
	assert.Equal(t, "root-runtime-beta", reflect.ValueOf(reusedPtr).Elem().FieldByName("Slug").String())
	assert.Equal(t, "Replacement Beta", reflect.ValueOf(reusedPtr).Elem().FieldByName("Label").String())

	// Replacement index works.
	sliceType := reflect.SliceOf(reflect.PointerTo(runtimeType))
	repResults := reflect.New(sliceType)
	err = node2.Find(ctx, "Group", "root-group-replacement", repResults.Interface())
	require.NoError(t, err)
	repSl, repN := runtimeSliceFromPtr(repResults)
	require.Equal(t, 1, repN)
	assert.Equal(t, "Replacement Beta", repSl.Index(0).Elem().FieldByName("Label").String())
}

// ------------- Q. Extended fixture baseline integrity -------------

func TestCompatibility_ExtendedFixtureBaselineIntegrity(t *testing.T) {
	path := copyFixture(t)
	db := openFixture(t, path)
	defer assertClose(t, db)

	ctx := context.Background()
	m := readManifest(t)

	// Four CompatibilityUser root records remain exact.
	var users []CompatibilityUser
	err := db.All(ctx, &users)
	require.NoError(t, err)
	require.Len(t, users, 4)

	byID := make(map[uint64]CompatibilityUser, 4)
	for _, u := range users {
		byID[u.ID] = u
	}
	for _, r := range m.RootRecords {
		u := byID[r.ID]
		assert.Equal(t, r.Email, u.Email)
		assert.Equal(t, r.Team, u.Team)
		assert.Equal(t, r.Name, u.Name)
		assert.Equal(t, r.Revision, u.Revision)
	}

	// Next root CompatibilityUser ID remains 5.
	newUser := CompatibilityUser{
		Email:    "extended@example.test",
		Team:     "team-ext",
		Name:     "Ext",
		Revision: 1,
	}
	err = db.Save(ctx, &newUser)
	require.NoError(t, err)
	assert.Equal(t, uint64(5), newUser.ID)

	// Root Team and Email indexes remain valid.
	var blue []CompatibilityUser
	err = db.Find(ctx, "Team", "team-blue", &blue)
	require.NoError(t, err)
	require.Len(t, blue, 2)
	assert.Equal(t, "alice@example.test", blue[0].Email)
	assert.Equal(t, "bob@example.test", blue[1].Email)

	for email, id := range m.Indexes.Email.Lookups {
		var u CompatibilityUser
		err = db.One(ctx, "Email", email, &u)
		require.NoError(t, err)
		assert.Equal(t, id, u.ID)
	}

	// Nested CompatibilityUser remains exact.
	nested := db.From("tenant", "acme")
	var nestedUsers []CompatibilityUser
	err = nested.All(ctx, &nestedUsers)
	require.NoError(t, err)
	require.Len(t, nestedUsers, 1)
	assert.Equal(t, "nested@example.test", nestedUsers[0].Email)
	assert.Equal(t, "team-nested", nestedUsers[0].Team)
	assert.Equal(t, "Nested User", nestedUsers[0].Name)
	assert.Equal(t, 5, nestedUsers[0].Revision)

	// Root and nested KV remain exact.
	var theme string
	err = db.Get(ctx, "settings", "theme", &theme)
	require.NoError(t, err)
	assert.Equal(t, "dark", theme)

	var retries int
	err = db.Get(ctx, "settings", "retries", &retries)
	require.NoError(t, err)
	assert.Equal(t, 3, retries)

	var region string
	err = nested.Get(ctx, "settings", "region", &region)
	require.NoError(t, err)
	assert.Equal(t, "us-test-1", region)
}
