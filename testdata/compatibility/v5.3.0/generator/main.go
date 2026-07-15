// generator creates a reproducible Rainstorm v5.3.0 baseline fixture
// for compatibility testing in Rainstorm v6.
//
// Run from the generator directory:
//
//	go run .   (creates ../baseline.db)
//
// The fixture is not byte-for-byte deterministic across runs because
// raw bbolt on-disk layout may vary. Compatibility is verified through
// the manifest and behavioral tests, not byte-level comparison.
package main

import (
	"errors"
	"fmt"
	"os"
	"reflect"

	rainstorm "github.com/AndersonBargas/rainstorm/v5"
)

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
// This proves v5 BucketNamer records are preserved by v6.
type CompatibilityNamedEvent struct {
	ID       uint64 `rainstorm:"id,increment"`
	Code     string `rainstorm:"unique"`
	Category string `rainstorm:"index"`
	Message  string
}

func (CompatibilityNamedEvent) RainstormBucketName() string {
	return "compatibility_named_events"
}

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	// Print success only after run() has returned without error,
	// which means Close succeeded (the deferred Close in run runs before return).
	fmt.Println("fixture written successfully")
}

func run() (err error) {
	target := ".." + string(os.PathSeparator) + "baseline.db"

	if err := os.Remove(target); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("remove existing fixture: %w", err)
	}

	db, err := rainstorm.Open(target)
	if err != nil {
		return fmt.Errorf("open: %w", err)
	}
	defer func() {
		if closeErr := db.Close(); closeErr != nil {
			err = errors.Join(err, fmt.Errorf("close: %w", closeErr))
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
			return fmt.Errorf("save %d: %w", i, err)
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
		return fmt.Errorf("nested save: %w", err)
	}
	fmt.Printf("nested user  : id=%d email=%s team=%s\n",
		nestedUser.ID, nestedUser.Email, nestedUser.Team)

	// Root KV entries.
	if err := db.Set("settings", "theme", "dark"); err != nil {
		return fmt.Errorf("kv set theme: %w", err)
	}
	if err := db.Set("settings", "retries", 3); err != nil {
		return fmt.Errorf("kv set retries: %w", err)
	}
	fmt.Println("root  kv     : settings/theme=dark settings/retries=3")

	// Nested KV entry.
	if err := nestedNode.Set("settings", "region", "us-test-1"); err != nil {
		return fmt.Errorf("nested kv set: %w", err)
	}
	fmt.Println("nested kv    : settings/region=us-test-1")

	// ===================================================================
	// R6.5B1 additions
	// ===================================================================

	// A. BucketNamer named type.
	events := []CompatibilityNamedEvent{
		{Code: "event-alpha", Category: "audit", Message: "Alpha"},
		{Code: "event-beta", Category: "audit", Message: "Beta"},
		{Code: "event-gamma", Category: "system", Message: "Gamma"},
	}
	for i := range events {
		if err := db.Save(&events[i]); err != nil {
			return fmt.Errorf("named event %d: %w", i, err)
		}
		fmt.Printf("named event  : id=%d code=%s category=%s\n",
			events[i].ID, events[i].Code, events[i].Category)
	}

	// B. Runtime-generated type through explicit From.
	runtimeExplicitType := reflect.StructOf([]reflect.StructField{
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
			return fmt.Errorf("runtime explicit %d: %w", i, err)
		}
		runtimeExplicitIDs[i] = val.Elem().FieldByName("ID").Uint()
		fmt.Printf("runtime expl %d: id=%d slug=%s group=%s\n",
			i+1, runtimeExplicitIDs[i], r.slug, r.group)
	}

	// C. Runtime-generated type using node root as data bucket.
	runtimeRootDataType := reflect.StructOf([]reflect.StructField{
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
			return fmt.Errorf("runtime root-data %d: %w", i, err)
		}
		runtimeRootDataIDs[i] = val.Elem().FieldByName("ID").Uint()
		fmt.Printf("runtime root  %d: id=%d slug=%s group=%s\n",
			i+1, runtimeRootDataIDs[i], r.slug, r.group)
	}

	return nil
}
