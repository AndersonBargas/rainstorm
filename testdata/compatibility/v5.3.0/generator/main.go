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

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
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
	fmt.Println("fixture written to", target)
	return nil
}
