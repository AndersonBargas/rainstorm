// generator creates reproducible Rainstorm v5.3.0 baseline and codec fixtures
// for compatibility testing in Rainstorm v6.
//
// Run from the generator directory:
//
//	go run .   (creates ../baseline.db and ../codecs/*.db)
//
// The fixtures are not byte-for-byte deterministic across runs because
// raw bbolt on-disk layout may vary. Compatibility is verified through
// the manifest and behavioral tests, not byte-level comparison.
package main

import (
	"encoding/base64"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"

	rainstorm "github.com/AndersonBargas/rainstorm/v5"
	"github.com/AndersonBargas/rainstorm/v5/codec"
	"github.com/AndersonBargas/rainstorm/v5/codec/aes"
	"github.com/AndersonBargas/rainstorm/v5/codec/gob"
	"github.com/AndersonBargas/rainstorm/v5/codec/json"
	"github.com/AndersonBargas/rainstorm/v5/codec/msgpack"
	"github.com/AndersonBargas/rainstorm/v5/codec/sereal"
)

// -- shared schema types ---------------------------------------------------

// CompatibilityUser matches the schema used by v6 compatibility tests.
// Tags follow the v5.3.0 syntax verified against the v5.3.0 extract.go.
type CompatibilityUser struct {
	ID       uint64 `rainstorm:"id,increment"`
	Email    string `rainstorm:"unique"`
	Team     string `rainstorm:"index"`
	Name     string
	Revision int
}

// CompatibilityNamedEvent uses a custom bucket name via BucketNamer.
type CompatibilityNamedEvent struct {
	ID       uint64 `rainstorm:"id,increment"`
	Code     string `rainstorm:"unique"`
	Category string `rainstorm:"index"`
	Message  string
}

func (CompatibilityNamedEvent) RainstormBucketName() string {
	return "compatibility_named_events"
}

// CodecCompatibilityRecord is the shared schema for codec-specific fixtures.
type CodecCompatibilityRecord struct {
	ID       uint64 `rainstorm:"id,increment"`
	Key      string `rainstorm:"unique"`
	Category string `rainstorm:"index"`
	Name     string
	Revision int
}

// -- AES test key (non-secret, test-only, 16-byte AES-128) -----------------

// testAESKeyB64 is a base64-encoded fixed key used only for compatibility tests.
// It is NOT a production secret.
const testAESKeyB64 = "xkBTXc1wn0C/aL31u9SA7g=="

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	fmt.Println("fixtures written successfully")
}

func run() (err error) {
	if err := runBaseline(); err != nil {
		return err
	}
	if err := runCodecs(); err != nil {
		return err
	}
	return nil
}

// -- baseline fixture (unchanged from R6.5B1) ------------------------------

func runBaseline() (err error) {
	target := filepath.Join("..", "baseline.db")

	if err := os.Remove(target); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("baseline: remove existing: %w", err)
	}

	db, err := rainstorm.Open(target)
	if err != nil {
		return fmt.Errorf("baseline: open: %w", err)
	}
	defer func() {
		if closeErr := db.Close(); closeErr != nil {
			err = errors.Join(err, fmt.Errorf("baseline: close: %w", closeErr))
		}
	}()

	// Root records.
	users := []CompatibilityUser{
		{Email: "alice@example.test", Team: "team-blue", Name: "Alice", Revision: 1},
		{Email: "bob@example.test", Team: "team-blue", Name: "Bob", Revision: 1},
		{Email: "carol@example.test", Team: "team-red", Name: "Carol", Revision: 2},
		{Email: "dave@example.test", Team: "team-green", Name: "Dave", Revision: 3},
	}

	for i := range users {
		if err := db.Save(&users[i]); err != nil {
			return fmt.Errorf("baseline: save %d: %w", i, err)
		}
		fmt.Printf("root  user %d: id=%d email=%s team=%s\n",
			i+1, users[i].ID, users[i].Email, users[i].Team)
	}

	// Nested record in tenant/acme.
	nestedNode := db.From("tenant", "acme")
	nestedUser := CompatibilityUser{
		Email:    "nested@example.test",
		Team:     "team-nested",
		Name:     "Nested User",
		Revision: 5,
	}
	if err := nestedNode.Save(&nestedUser); err != nil {
		return fmt.Errorf("baseline: nested save: %w", err)
	}
	fmt.Printf("nested user  : id=%d email=%s team=%s\n",
		nestedUser.ID, nestedUser.Email, nestedUser.Team)

	// Root KV entries.
	if err := db.Set("settings", "theme", "dark"); err != nil {
		return fmt.Errorf("baseline: kv set theme: %w", err)
	}
	if err := db.Set("settings", "retries", 3); err != nil {
		return fmt.Errorf("baseline: kv set retries: %w", err)
	}
	fmt.Println("root  kv     : settings/theme=dark settings/retries=3")

	// Nested KV entry.
	if err := nestedNode.Set("settings", "region", "us-test-1"); err != nil {
		return fmt.Errorf("baseline: nested kv set: %w", err)
	}
	fmt.Println("nested kv    : settings/region=us-test-1")

	// R6.5B1 additions.

	// A. BucketNamer named type.
	events := []CompatibilityNamedEvent{
		{Code: "event-alpha", Category: "audit", Message: "Alpha"},
		{Code: "event-beta", Category: "audit", Message: "Beta"},
		{Code: "event-gamma", Category: "system", Message: "Gamma"},
	}
	for i := range events {
		if err := db.Save(&events[i]); err != nil {
			return fmt.Errorf("baseline: named event %d: %w", i, err)
		}
		fmt.Printf("named event  : id=%d code=%s category=%s\n",
			events[i].ID, events[i].Code, events[i].Category)
	}

	// B. Runtime-generated type through explicit From.
	runtimeExplicitType := reflect.StructOf([]reflect.StructField{
		{Name: "ID", Type: reflect.TypeOf(uint64(0)), Tag: reflect.StructTag(`rainstorm:"id,increment"`)},
		{Name: "Slug", Type: reflect.TypeOf(""), Tag: reflect.StructTag(`rainstorm:"unique"`)},
		{Name: "Group", Type: reflect.TypeOf(""), Tag: reflect.StructTag(`rainstorm:"index"`)},
		{Name: "Label", Type: reflect.TypeOf("")},
		{Name: "Revision", Type: reflect.TypeOf(0)},
	})

	runtimeExplicitNode := db.From("runtime", "explicit")
	rtRecs := []struct {
		slug, group, label string
		revision           int
	}{
		{slug: "runtime-alpha", group: "group-shared", label: "Runtime Alpha", revision: 1},
		{slug: "runtime-beta", group: "group-shared", label: "Runtime Beta", revision: 1},
		{slug: "runtime-gamma", group: "group-other", label: "Runtime Gamma", revision: 2},
	}
	runtimeExplicitIDs := make([]uint64, len(rtRecs))
	for i, r := range rtRecs {
		val := reflect.New(runtimeExplicitType)
		val.Elem().FieldByName("Slug").SetString(r.slug)
		val.Elem().FieldByName("Group").SetString(r.group)
		val.Elem().FieldByName("Label").SetString(r.label)
		val.Elem().FieldByName("Revision").SetInt(int64(r.revision))
		if err := runtimeExplicitNode.Save(val.Interface()); err != nil {
			return fmt.Errorf("baseline: runtime explicit %d: %w", i, err)
		}
		runtimeExplicitIDs[i] = val.Elem().FieldByName("ID").Uint()
		fmt.Printf("runtime expl %d: id=%d slug=%s group=%s\n",
			i+1, runtimeExplicitIDs[i], r.slug, r.group)
	}

	// C. Runtime-generated type using node root as data bucket.
	runtimeRootDataType := reflect.StructOf([]reflect.StructField{
		{Name: "ID", Type: reflect.TypeOf(uint64(0)), Tag: reflect.StructTag(`rainstorm:"id,increment"`)},
		{Name: "Slug", Type: reflect.TypeOf(""), Tag: reflect.StructTag(`rainstorm:"unique"`)},
		{Name: "Group", Type: reflect.TypeOf(""), Tag: reflect.StructTag(`rainstorm:"index"`)},
		{Name: "Label", Type: reflect.TypeOf("")},
		{Name: "Revision", Type: reflect.TypeOf(0)},
	})

	runtimeRootDataNode := db.From("runtime", "root-data")
	rtRootRecs := []struct {
		slug, group, label string
		revision           int
	}{
		{slug: "root-runtime-alpha", group: "root-group", label: "Root Runtime Alpha", revision: 3},
		{slug: "root-runtime-beta", group: "root-group", label: "Root Runtime Beta", revision: 4},
	}
	runtimeRootDataIDs := make([]uint64, len(rtRootRecs))
	for i, r := range rtRootRecs {
		val := reflect.New(runtimeRootDataType)
		val.Elem().FieldByName("Slug").SetString(r.slug)
		val.Elem().FieldByName("Group").SetString(r.group)
		val.Elem().FieldByName("Label").SetString(r.label)
		val.Elem().FieldByName("Revision").SetInt(int64(r.revision))
		if err := runtimeRootDataNode.Save(val.Interface()); err != nil {
			return fmt.Errorf("baseline: runtime root-data %d: %w", i, err)
		}
		runtimeRootDataIDs[i] = val.Elem().FieldByName("ID").Uint()
		fmt.Printf("runtime root  %d: id=%d slug=%s group=%s\n",
			i+1, runtimeRootDataIDs[i], r.slug, r.group)
	}

	return nil
}

// -- codec fixtures (R6.5B2) -----------------------------------------------

func runCodecs() (err error) {
	codecsDir := filepath.Join("..", "codecs")
	if err := os.MkdirAll(codecsDir, 0755); err != nil {
		return fmt.Errorf("codecs: mkdir: %w", err)
	}

	testKey, err := base64.StdEncoding.DecodeString(testAESKeyB64)
	if err != nil {
		return fmt.Errorf("codecs: decode AES test key: %w", err)
	}

	// Non-AES codecs.
	plainCodecs := []struct {
		name     string
		filename string
		codec    codec.MarshalUnmarshaler
	}{
		{name: "gob", filename: "gob.db", codec: gob.Codec},
		{name: "msgpack", filename: "msgpack.db", codec: msgpack.Codec},
		{name: "sereal", filename: "sereal.db", codec: sereal.Codec},
	}
	for _, c := range plainCodecs {
		target := filepath.Join(codecsDir, c.filename)
		if err := createCodecFixture(target, c.name, c.codec); err != nil {
			return fmt.Errorf("codecs %s: %w", c.name, err)
		}
	}

	// AES fixture.
	aesCodec, err := aes.NewAES(json.Codec, testKey)
	if err != nil {
		return fmt.Errorf("codecs aes: create codec: %w", err)
	}
	aesTarget := filepath.Join(codecsDir, "aes.db")
	if err := createCodecFixture(aesTarget, "aes", aesCodec); err != nil {
		return fmt.Errorf("codecs aes: %w", err)
	}

	return nil
}

// createCodecFixture removes any existing target file, opens a fresh v5 DB
// with the given codec, populates it, and closes it. Populate and Close
// errors are preserved with errors.Join. Success is printed only after Close.
func createCodecFixture(target, codecName string, c codec.MarshalUnmarshaler) (err error) {
	if remErr := os.Remove(target); remErr != nil && !errors.Is(remErr, os.ErrNotExist) {
		return fmt.Errorf("remove existing: %w", remErr)
	}

	db, openErr := rainstorm.Open(target, rainstorm.Codec(c))
	if openErr != nil {
		return fmt.Errorf("open: %w", openErr)
	}

	popErr := populateCodecDB(db, codecName)
	closeErr := db.Close()
	err = errors.Join(popErr, closeErr)
	if err == nil {
		fmt.Printf("codec %-10s: %s\n", codecName, target)
	}
	return err
}

func populateCodecDB(db *rainstorm.DB, codecName string) error {
	records := []CodecCompatibilityRecord{
		{Key: "alpha", Category: "shared", Name: "Alpha", Revision: 1},
		{Key: "beta", Category: "shared", Name: "Beta", Revision: 2},
		{Key: "gamma", Category: "other", Name: "Gamma", Revision: 3},
	}

	for i := range records {
		if err := db.Save(&records[i]); err != nil {
			return fmt.Errorf("save record %d: %w", i, err)
		}
		if records[i].ID != uint64(i+1) {
			return fmt.Errorf("record %d: expected ID %d, got %d", i, i+1, records[i].ID)
		}
		fmt.Printf("  %s rec %d: id=%d key=%s category=%s\n",
			codecName, i+1, records[i].ID, records[i].Key, records[i].Category)
	}

	if err := db.Set("settings", "name", codecName+"-fixture"); err != nil {
		return fmt.Errorf("kv set name: %w", err)
	}
	if err := db.Set("settings", "revision", 1); err != nil {
		return fmt.Errorf("kv set revision: %w", err)
	}
	fmt.Printf("  %s kv  : settings/name=%s-fixture settings/revision=1\n", codecName, codecName)

	return nil
}
