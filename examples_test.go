package rainstorm_test

import (
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/AndersonBargas/rainstorm/v6"
	"github.com/AndersonBargas/rainstorm/v6/codec/gob"
	bolt "go.etcd.io/bbolt"
)

func ExampleDB_Save() {
	dir, _ := os.MkdirTemp(os.TempDir(), "rainstorm")
	defer os.RemoveAll(dir)

	ctx := context.Background()

	type User struct {
		ID        int    `rainstorm:"id,increment"` // the increment tag will auto-increment integer IDs without existing values.
		Group     string `rainstorm:"index"`
		Email     string `rainstorm:"unique"`
		Name      string
		Age       int       `rainstorm:"index"`
		CreatedAt time.Time `rainstorm:"index"`
	}

	// Open takes an optional list of options as the last argument.
	db, _ := rainstorm.Open(ctx, filepath.Join(dir, "rainstorm.db"), rainstorm.Codec(gob.Codec))
	defer db.Close()

	user := User{
		Group:     "staff",
		Email:     "john@provider.com",
		Name:      "John",
		Age:       21,
		CreatedAt: time.Now(),
	}

	err := db.Save(ctx, &user)

	if err != nil {
		log.Fatal(err)
	}

	fmt.Println(user.ID)

	user2 := user
	user2.ID = 0

	// Save will fail because of the unique constraint on Email
	err = db.Save(ctx, &user2)
	fmt.Println(err)

	// Output:
	// 1
	// already exists
}

func ExampleDB_One() {
	dir, db := prepareDB()
	defer os.RemoveAll(dir)
	defer db.Close()

	ctx := context.Background()

	var user User

	err := db.One(ctx, "Email", "john@provider.com", &user)

	if err != nil {
		log.Fatal(err)
	}

	// also works on unindexed fields
	err = db.One(ctx, "Name", "John", &user)

	if err != nil {
		log.Fatal(err)
	}

	err = db.One(ctx, "Name", "Jack", &user)
	fmt.Println(err)

	// Output:
	// not found
}

func ExampleDB_Find() {
	dir, db := prepareDB()
	defer os.RemoveAll(dir)
	defer db.Close()

	ctx := context.Background()

	var users []User
	err := db.Find(ctx, "Group", "staff", &users)

	if err != nil {
		log.Fatal(err)
	}

	fmt.Println("Found", len(users))

	// Output:
	// Found 3
}

func ExampleDB_All() {
	dir, db := prepareDB()
	defer os.RemoveAll(dir)
	defer db.Close()

	ctx := context.Background()

	var users []User
	err := db.All(ctx, &users)

	if err != nil {
		log.Fatal(err)
	}

	fmt.Println("Found", len(users))

	// Output:
	// Found 3
}

func ExampleDB_AllByIndex() {
	dir, db := prepareDB()
	defer os.RemoveAll(dir)
	defer db.Close()

	ctx := context.Background()

	var users []User
	err := db.AllByIndex(ctx, "CreatedAt", &users)

	if err != nil {
		log.Fatal(err)
	}

	fmt.Println("Found", len(users))

	// Output:
	// Found 3
}

func ExampleDB_Range() {
	dir, db := prepareDB()
	defer os.RemoveAll(dir)
	defer db.Close()

	ctx := context.Background()

	var users []User
	err := db.Range(ctx, "Age", 21, 22, &users)

	if err != nil {
		log.Fatal(err)
	}

	fmt.Println("Found", len(users))

	// Output:
	// Found 2
}

func ExampleLimit() {
	dir, db := prepareDB()
	defer os.RemoveAll(dir)
	defer db.Close()

	ctx := context.Background()

	var users []User
	err := db.All(ctx, &users, rainstorm.Limit(2))

	if err != nil {
		log.Fatal(err)
	}

	fmt.Println("Found", len(users))

	// Output:
	// Found 2
}

func ExampleSkip() {
	dir, db := prepareDB()
	defer os.RemoveAll(dir)
	defer db.Close()

	ctx := context.Background()

	var users []User
	err := db.All(ctx, &users, rainstorm.Skip(1))

	if err != nil {
		log.Fatal(err)
	}

	fmt.Println("Found", len(users))

	// Output:
	// Found 2
}

func ExampleUseDB() {
	dir, _ := os.MkdirTemp(os.TempDir(), "rainstorm")
	defer os.RemoveAll(dir)

	ctx := context.Background()

	bDB, err := bolt.Open(filepath.Join(dir, "bolt.db"), 0600, &bolt.Options{Timeout: 10 * time.Second})
	if err != nil {
		log.Fatal(err)
	}

	db, _ := rainstorm.Open(ctx, "", rainstorm.UseDB(bDB))
	defer db.Close()

	err = db.Save(ctx, &User{ID: 10})
	if err != nil {
		log.Fatal(err)
	}

	var user User
	err = db.One(ctx, "ID", 10, &user)
	fmt.Println(err)

	// Output:
	// <nil>
}

func ExampleDB_DeleteStruct() {
	dir, db := prepareDB()
	defer os.RemoveAll(dir)
	defer db.Close()

	ctx := context.Background()

	var user User

	err := db.One(ctx, "ID", 1, &user)

	if err != nil {
		log.Fatal(err)
	}

	err = db.DeleteStruct(ctx, &user)
	fmt.Println(err)

	// Output:
	// <nil>
}

func ExampleDB_Begin() {
	dir, db := prepareDB()
	defer os.RemoveAll(dir)
	defer db.Close()

	ctx := context.Background()

	// both start out with a balance of 10000 cents
	var account1, account2 Account

	tx, err := db.Begin(ctx, true)

	if err != nil {
		log.Fatal(err)
	}
	defer tx.Rollback()

	err = tx.One(ctx, "ID", 1, &account1)

	if err != nil {
		log.Fatal(err)
	}

	err = tx.One(ctx, "ID", 2, &account2)

	if err != nil {
		log.Fatal(err)
	}

	account1.Amount -= 1000
	account2.Amount += 1000

	err = tx.Save(ctx, &account1)

	if err != nil {
		log.Fatal(err)
	}

	err = tx.Save(ctx, &account2)

	if err != nil {
		log.Fatal(err)
	}

	tx.Commit(ctx)

	var account1Reloaded, account2Reloaded Account

	err = db.One(ctx, "ID", 1, &account1Reloaded)

	if err != nil {
		log.Fatal(err)
	}

	err = db.One(ctx, "ID", 2, &account2Reloaded)

	if err != nil {
		log.Fatal(err)
	}

	fmt.Println("Amount in account 1:", account1Reloaded.Amount)
	fmt.Println("Amount in account 2:", account2Reloaded.Amount)

	// Output:
	// Amount in account 1: 9000
	// Amount in account 2: 11000
}

func ExampleDB_From() {
	dir, db := prepareDB()
	defer os.RemoveAll(dir)
	defer db.Close()

	ctx := context.Background()

	// Create some sub buckets to partition the data.
	privateNotes := db.From("notes", "private")
	workNotes := db.From("notes", "work")

	err := privateNotes.Save(ctx, &Note{ID: "private1", Text: "This is some private text."})

	if err != nil {
		log.Fatal(err)
	}

	err = workNotes.Save(ctx, &Note{ID: "work1", Text: "Work related."})

	if err != nil {
		log.Fatal(err)
	}

	var privateNote, workNote, personalNote Note

	err = privateNotes.One(ctx, "ID", "work1", &workNote)

	// Not found: Wrong bucket.
	fmt.Println(err)

	err = workNotes.One(ctx, "ID", "work1", &workNote)

	if err != nil {
		log.Fatal(err)
	}

	err = privateNotes.One(ctx, "ID", "private1", &privateNote)

	if err != nil {
		log.Fatal(err)
	}

	fmt.Println(workNote.Text)
	fmt.Println(privateNote.Text)

	// These can be nested further if needed:
	personalNotes := privateNotes.From("personal")
	err = personalNotes.Save(ctx, &Note{ID: "personal1", Text: "This is some very personal text."})

	if err != nil {
		log.Fatal(err)
	}

	err = personalNotes.One(ctx, "ID", "personal1", &personalNote)

	if err != nil {
		log.Fatal(err)
	}

	fmt.Println(personalNote.Text)

	// Output:
	// not found
	// Work related.
	// This is some private text.
	// This is some very personal text.
}

func ExampleDB_Drop() {
	dir, db := prepareDB()
	defer os.RemoveAll(dir)
	defer db.Close()

	ctx := context.Background()

	var user User

	err := db.One(ctx, "Email", "john@provider.com", &user)

	if err != nil {
		log.Fatal(err)
	}

	err = db.Drop(ctx, "User")
	if err != nil {
		log.Fatal(err)
	}

	// One only works for indexed fields.
	err = db.One(ctx, "Email", "john@provider.com", &user)
	fmt.Println(err)

	// Output:
	// not found
}

func ExampleNode_PrefixScan() {
	dir, db := prepareDB()
	defer os.RemoveAll(dir)
	defer db.Close()

	ctx := context.Background()

	// The PrefixScan method is available on both DB and Node.
	// This example shows the usage on Node.
	// The Node notes will be the top-level bucket.
	notes := db.From("notes")

	// Partition the notes in one bucket per month.
	for i := 2014; i <= 2016; i++ {
		for j := 1; j <= 12; j++ {
			bucket := notes.From(fmt.Sprintf("%d%02d", i, j))
			numNotes := 2

			// Add some variation.
			if j%3 == 0 {
				numNotes = 3
			}

			for k := 0; k < numNotes; k++ {
				noteID := fmt.Sprintf("%d-%d", j, k)
				if err := bucket.Save(ctx, &Note{ID: noteID, Text: fmt.Sprintf("Note %s", noteID)}); err != nil {
					log.Fatal(err)
				}
			}
		}
	}

	// There are now three years worth of notes. Let's look at 2016:
	nodes, err := notes.PrefixScan(ctx, "2016")
	if err != nil {
		log.Fatal(err)
	}

	fmt.Println("Note buckets in 2016:", len(nodes))

	marchNodes, err := notes.PrefixScan(ctx, "201603")
	if err != nil {
		log.Fatal(err)
	}
	march := marchNodes[0]

	// The two below points to the same bucket:
	fmt.Println("Bucket", march.Bucket()[1])
	fmt.Println("Bucket", nodes[2].Bucket()[1])

	count, err := march.Count(ctx, &Note{})

	if err != nil {
		log.Fatal(err)
	}

	fmt.Println("Notes in March:", count)

	// Output:
	//Note buckets in 2016: 12
	//Bucket 201603
	//Bucket 201603
	//Notes in March: 3

}

func ExampleNode_RangeScan() {
	dir, db := prepareDB()
	defer os.RemoveAll(dir)
	defer db.Close()

	ctx := context.Background()

	// The RangeScan method is available on both DB and Node.
	// This example shows the usage on Node.
	// The Node notes will be the top-level bucket.
	notes := db.From("notes")

	// Partition the notes in one bucket per month.
	for i := 2013; i <= 2016; i++ {
		for j := 1; j <= 12; j++ {
			for k := 0; k < 3; k++ {
				// Must left-pad the month so it is sortable.
				bucket := notes.From(fmt.Sprintf("%d%02d", i, j))
				noteID := fmt.Sprintf("%d-%d", j, k)
				if err := bucket.Save(ctx, &Note{ID: noteID, Text: fmt.Sprintf("Note %s", noteID)}); err != nil {
					log.Fatal(err)
				}
			}
		}
	}

	// There are now four years worth of notes. Let's look at first half of 2014:
	nodes, err := notes.RangeScan(ctx, "201401", "201406")
	if err != nil {
		log.Fatal(err)
	}

	fmt.Println("Note buckets in first half of 2014:", len(nodes))

	notesCount, err := nodes[0].Count(ctx, &Note{})

	if err != nil {
		log.Fatal(err)
	}

	fmt.Println("Notes in bucket:", notesCount)

	// Output:
	// Note buckets in first half of 2014: 6
	// Notes in bucket: 3
}

type User struct {
	ID        int    `rainstorm:"id,increment"`
	Group     string `rainstorm:"index"`
	Email     string `rainstorm:"unique"`
	Name      string
	Age       int       `rainstorm:"index"`
	CreatedAt time.Time `rainstorm:"index"`
}

type Account struct {
	ID     int   `rainstorm:"id,increment"`
	Amount int64 // amount in cents
}

type Note struct {
	ID   string `rainstorm:"id"`
	Text string
}

func prepareDB() (string, *rainstorm.DB) {
	dir, _ := os.MkdirTemp(os.TempDir(), "rainstorm")
	ctx := context.Background()
	db, _ := rainstorm.Open(ctx, filepath.Join(dir, "rainstorm.db"))

	for i, name := range []string{"John", "Eric", "Dilbert"} {
		email := strings.ToLower(name + "@provider.com")
		user := User{
			Group:     "staff",
			Email:     email,
			Name:      name,
			Age:       21 + i,
			CreatedAt: time.Now(),
		}
		err := db.Save(ctx, &user)

		if err != nil {
			log.Fatal(err)
		}
	}

	for i := int64(0); i < 10; i++ {
		account := Account{Amount: 10000}

		err := db.Save(ctx, &account)

		if err != nil {
			log.Fatal(err)
		}
	}

	return dir, db
}
