package q_test

import (
	"context"
	"fmt"
	"log"

	"time"

	"os"

	"path/filepath"
	"strings"

	"github.com/AndersonBargas/rainstorm/v6"
	"github.com/AndersonBargas/rainstorm/v6/q"
)

func ExampleRe() {
	dir, db := prepareDB()
	defer os.RemoveAll(dir)
	defer db.Close()

	ctx := context.Background()

	var users []User

	// Find all users with name that starts with the letter D.
	if err := db.Select(q.Re("Name", "^D")).Find(ctx, &users); err != nil {
		log.Println("error: Select failed:", err)
		return
	}

	// Donald and Dilbert
	fmt.Println("Found", len(users), "users.")

	// Output:
	// Found 2 users.
}

type User struct {
	ID        int    `rainstorm:"id,increment"`
	Group     string `rainstorm:"index"`
	Email     string `rainstorm:"unique"`
	Name      string
	Age       int       `rainstorm:"index"`
	CreatedAt time.Time `rainstorm:"index"`
}

func prepareDB() (string, *rainstorm.DB) {
	dir, err := os.MkdirTemp(os.TempDir(), "rainstorm")
	if err != nil {
		log.Fatal(err)
	}
	ctx := context.Background()
	db, err := rainstorm.Open(ctx, filepath.Join(dir, "rainstorm.db"))
	if err != nil {
		log.Fatal(err)
	}

	for i, name := range []string{"John", "Norm", "Donald", "Eric", "Dilbert"} {
		email := strings.ToLower(name + "@provider.com")
		user := User{
			Group:     "staff",
			Email:     email,
			Name:      name,
			Age:       21 + i,
			CreatedAt: time.Now(),
		}
		err = db.Save(ctx, &user)

		if err != nil {
			log.Fatal(err)
		}
	}

	return dir, db
}
