package rainstorm

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	bolt "go.etcd.io/bbolt"
)

func TestFind(t *testing.T) {
	db, cleanup := createDB(t)
	defer cleanup()

	ctx := context.Background()

	for i := 0; i < 100; i++ {
		w := User{Name: "John", ID: i + 1, Slug: fmt.Sprintf("John%d", i+1)}

		if i%2 == 0 {
			w.Group = "staff"
		} else {
			w.Group = "normal"
		}

		err := db.Save(ctx, &w)
		require.NoError(t, err)
	}

	err := db.Find(ctx, "Name", "John", &User{})
	require.Error(t, err)
	require.ErrorIs(t, err, ErrSlicePtrNeeded)

	err = db.Find(ctx, "Name", "John", &[]struct {
		Name string
		ID   int
	}{})
	require.Error(t, err)
	require.ErrorIs(t, err, ErrNoName)

	notTheRightUsers := []UniqueNameUser{}

	err = db.Find(ctx, "Name", "John", &notTheRightUsers)
	require.Error(t, err)
	require.EqualError(t, err, "not found")

	users := []User{}

	err = db.Find(ctx, "unexportedField", "John", &users)
	require.Error(t, err)
	require.EqualError(t, err, "field unexportedField not found")

	err = db.Find(ctx, "DateOfBirth", "John", &users)
	require.Error(t, err)
	require.ErrorIs(t, err, ErrNotFound)

	err = db.Find(ctx, "Group", "staff", &users)
	require.NoError(t, err)
	require.Len(t, users, 50)
	require.Equal(t, 1, users[0].ID)
	require.Equal(t, 99, users[49].ID)

	err = db.Find(ctx, "Group", "staff", &users, Reverse())
	require.NoError(t, err)
	require.Len(t, users, 50)
	require.Equal(t, 99, users[0].ID)
	require.Equal(t, 1, users[49].ID)

	err = db.Find(ctx, "Group", "admin", &users)
	require.Error(t, err)
	require.ErrorIs(t, err, ErrNotFound)

	err = db.Find(ctx, "Name", "John", users)
	require.Error(t, err)
	require.ErrorIs(t, err, ErrSlicePtrNeeded)

	err = db.Find(ctx, "Name", "John", &users)
	require.NoError(t, err)
	require.Len(t, users, 100)
	require.Equal(t, 1, users[0].ID)
	require.Equal(t, 100, users[99].ID)

	err = db.Find(ctx, "Name", "John", &users, Reverse())
	require.NoError(t, err)
	require.Len(t, users, 100)
	require.Equal(t, 100, users[0].ID)
	require.Equal(t, 1, users[99].ID)

	users = []User{}
	err = db.Find(ctx, "Slug", "John10", &users)
	require.NoError(t, err)
	require.Len(t, users, 1)
	require.Equal(t, 10, users[0].ID)

	users = []User{}
	err = db.Find(ctx, "Name", nil, &users)
	require.Error(t, err)
	require.ErrorIs(t, err, ErrNotFound)

	err = db.Find(ctx, "Name", "John", &users, Limit(10), Skip(20))
	require.NoError(t, err)
	require.Len(t, users, 10)
	require.Equal(t, 21, users[0].ID)
	require.Equal(t, 30, users[9].ID)

	err = db.Find(ctx, "Age", 10, &users)
	require.NoError(t, err)
}

func TestFindNil(t *testing.T) {
	db, cleanup := createDB(t)
	defer cleanup()

	ctx := context.Background()

	type User struct {
		ID        int        `rainstorm:"increment"`
		CreatedAt *time.Time `rainstorm:"index"`
		DeletedAt *time.Time `rainstorm:"unique"`
	}

	t1 := time.Now()
	for i := 0; i < 10; i++ {
		now := time.Now()
		var u User

		if i%2 == 0 {
			u.CreatedAt = &t1
			u.DeletedAt = &now
		}

		err := db.Save(ctx, &u)
		require.NoError(t, err)
	}

	var users []User
	err := db.Find(ctx, "CreatedAt", nil, &users)
	require.NoError(t, err)
	require.Len(t, users, 5)

	users = nil
	err = db.Find(ctx, "CreatedAt", t1, &users)
	require.NoError(t, err)
	require.Len(t, users, 5)

	users = nil
	err = db.Find(ctx, "DeletedAt", nil, &users)
	require.NoError(t, err)
	require.Len(t, users, 5)
}

func TestFindIntIndex(t *testing.T) {
	db, cleanup := createDB(t)
	defer cleanup()

	ctx := context.Background()

	type Score struct {
		ID    int    `rainstorm:"increment"`
		Score uint64 `rainstorm:"index"`
	}

	for i := 0; i < 10; i++ {
		w := Score{Score: uint64(i % 3)}
		err := db.Save(ctx, &w)
		require.NoError(t, err)
	}

	var scores []Score
	err := db.Find(ctx, "Score", 2, &scores)
	require.NoError(t, err)
	require.Len(t, scores, 3)
	require.Equal(t, []Score{
		{ID: 3, Score: 2},
		{ID: 6, Score: 2},
		{ID: 9, Score: 2},
	}, scores)
}

func TestAllByIndex(t *testing.T) {
	db, cleanup := createDB(t)
	defer cleanup()

	ctx := context.Background()

	for i := 0; i < 100; i++ {
		w := User{Name: "John", ID: i + 1, Slug: fmt.Sprintf("John%d", i+1), DateOfBirth: time.Now().Add(-time.Duration(i*10) * time.Minute)}
		err := db.Save(ctx, &w)
		require.NoError(t, err)
	}

	err := db.AllByIndex(ctx, "", nil)
	require.Error(t, err)
	require.ErrorIs(t, err, ErrSlicePtrNeeded)

	var users []User

	err = db.AllByIndex(ctx, "Unknown field", &users)
	require.Error(t, err)
	require.ErrorIs(t, err, ErrNotFound)

	err = db.AllByIndex(ctx, "DateOfBirth", &users)
	require.NoError(t, err)
	require.Len(t, users, 100)
	require.Equal(t, 100, users[0].ID)
	require.Equal(t, 1, users[99].ID)

	err = db.AllByIndex(ctx, "Name", &users)
	require.NoError(t, err)
	require.Len(t, users, 100)
	require.Equal(t, 1, users[0].ID)
	require.Equal(t, 100, users[99].ID)

	y := UniqueNameUser{Name: "Jake", ID: 200}
	err = db.Save(ctx, &y)
	require.NoError(t, err)

	var y2 []UniqueNameUser
	err = db.AllByIndex(ctx, "ID", &y2)
	require.NoError(t, err)
	require.Len(t, y2, 1)

	n := NestedID{}
	n.ID = "100"
	n.Name = "John"

	err = db.Save(ctx, &n)
	require.NoError(t, err)

	var n2 []NestedID
	err = db.AllByIndex(ctx, "ID", &n2)
	require.NoError(t, err)
	require.Len(t, n2, 1)

	err = db.AllByIndex(ctx, "Name", &users, Limit(10))
	require.NoError(t, err)
	require.Len(t, users, 10)
	require.Equal(t, 1, users[0].ID)
	require.Equal(t, 10, users[9].ID)

	err = db.AllByIndex(ctx, "Name", &users, Limit(200))
	require.NoError(t, err)
	require.Len(t, users, 100)
	require.Equal(t, 1, users[0].ID)
	require.Equal(t, 100, users[99].ID)

	err = db.AllByIndex(ctx, "Name", &users, Limit(-10))
	require.NoError(t, err)
	require.Len(t, users, 100)
	require.Equal(t, 1, users[0].ID)
	require.Equal(t, 100, users[99].ID)

	err = db.AllByIndex(ctx, "Name", &users, Skip(200))
	require.NoError(t, err)
	require.Len(t, users, 0)

	err = db.AllByIndex(ctx, "Name", &users, Skip(-10))
	require.NoError(t, err)
	require.Len(t, users, 100)
	require.Equal(t, 1, users[0].ID)
	require.Equal(t, 100, users[99].ID)

	err = db.AllByIndex(ctx, "ID", &users)
	require.NoError(t, err)
	require.Len(t, users, 100)
	require.Equal(t, 1, users[0].ID)
	require.Equal(t, 100, users[99].ID)

	err = db.AllByIndex(ctx, "ID", &users, Limit(10))
	require.NoError(t, err)
	require.Len(t, users, 10)
	require.Equal(t, 1, users[0].ID)
	require.Equal(t, 10, users[9].ID)

	err = db.AllByIndex(ctx, "ID", &users, Skip(10))
	require.NoError(t, err)
	require.Len(t, users, 90)
	require.Equal(t, 11, users[0].ID)
	require.Equal(t, 100, users[89].ID)

	err = db.AllByIndex(ctx, "Name", &users, Limit(10), Skip(10))
	require.NoError(t, err)
	require.Len(t, users, 10)
	require.Equal(t, 11, users[0].ID)
	require.Equal(t, 20, users[9].ID)

	err = db.AllByIndex(ctx, "Name", &users, Limit(10), Skip(10), Reverse())
	require.NoError(t, err)
	require.Len(t, users, 10)
	require.Equal(t, 90, users[0].ID)
	require.Equal(t, 81, users[9].ID)

	err = db.AllByIndex(ctx, "Age", &users, Limit(10))
	require.NoError(t, err)
	require.Len(t, users, 10)
}

func TestAll(t *testing.T) {
	db, cleanup := createDB(t)
	defer cleanup()

	ctx := context.Background()

	for i := 0; i < 100; i++ {
		w := User{Name: "John", ID: i + 1, Slug: fmt.Sprintf("John%d", i+1), DateOfBirth: time.Now().Add(-time.Duration(i*10) * time.Minute)}
		err := db.Save(ctx, &w)
		require.NoError(t, err)
	}

	var users []User

	err := db.All(ctx, &users)
	require.NoError(t, err)
	require.Len(t, users, 100)
	require.Equal(t, 1, users[0].ID)
	require.Equal(t, 100, users[99].ID)

	err = db.All(ctx, &users, Reverse())
	require.NoError(t, err)
	require.Len(t, users, 100)
	require.Equal(t, 100, users[0].ID)
	require.Equal(t, 1, users[99].ID)

	var users2 []*User

	err = db.All(ctx, &users2)
	require.NoError(t, err)
	require.Len(t, users2, 100)
	require.Equal(t, 1, users2[0].ID)
	require.Equal(t, 100, users2[99].ID)

	err = db.Save(ctx, &NestedID{
		ToEmbed: ToEmbed{ID: "id1"},
		Name:    "John",
	})
	require.NoError(t, err)

	err = db.Save(ctx, &NestedID{
		ToEmbed: ToEmbed{ID: "id2"},
		Name:    "Mike",
	})
	require.NoError(t, err)

	db.Save(ctx, &NestedID{
		ToEmbed: ToEmbed{ID: "id3"},
		Name:    "Steve",
	})
	require.NoError(t, err)

	var nested []NestedID
	err = db.All(ctx, &nested)
	require.NoError(t, err)
	require.Len(t, nested, 3)

	err = db.All(ctx, &users, Limit(10), Skip(10))
	require.NoError(t, err)
	require.Len(t, users, 10)
	require.Equal(t, 11, users[0].ID)
	require.Equal(t, 20, users[9].ID)

	err = db.All(ctx, &users, Limit(0), Skip(0))
	require.NoError(t, err)
	require.Len(t, users, 0)
}

func TestCount(t *testing.T) {
	db, cleanup := createDB(t)
	defer cleanup()

	ctx := context.Background()

	for i := 0; i < 100; i++ {
		w := User{Name: "John", ID: i + 1, Slug: fmt.Sprintf("John%d", i+1), DateOfBirth: time.Now().Add(-time.Duration(i*10) * time.Minute)}
		err := db.Save(ctx, &w)
		require.NoError(t, err)
	}

	count, err := db.Count(ctx, &User{})
	require.NoError(t, err)
	require.Equal(t, 100, count)

	w := User{Name: "John", ID: 101, Slug: fmt.Sprintf("John%d", 101), DateOfBirth: time.Now().Add(-time.Duration(101*10) * time.Minute)}
	err = db.Save(ctx, &w)
	require.NoError(t, err)

	count, err = db.Count(ctx, &User{})
	require.NoError(t, err)
	require.Equal(t, 101, count)

	err = db.WriteTransaction(ctx, func(txn Node) error {
		_, cerr := txn.Count(ctx, User{})
		require.ErrorIs(t, cerr, ErrStructPtrNeeded)

		c, cerr := txn.Count(ctx, &User{})
		require.NoError(t, cerr)
		require.Equal(t, 101, c)

		w2 := User{Name: "John", ID: 102, Slug: fmt.Sprintf("John%d", 102), DateOfBirth: time.Now().Add(-time.Duration(101*10) * time.Minute)}
		cerr = txn.Save(ctx, &w2)
		require.NoError(t, cerr)

		c, cerr = txn.Count(ctx, &User{})
		require.NoError(t, cerr)
		require.Equal(t, 102, c)

		return nil
	})
	require.NoError(t, err)
}

func TestCountEmpty(t *testing.T) {
	db, cleanup := createDB(t)
	defer cleanup()

	ctx := context.Background()

	user := &User{}
	err := db.Init(ctx, user)
	require.NoError(t, err)

	count, err := db.Count(ctx, user)
	require.Zero(t, count)
	require.NoError(t, err)
}

func TestOne(t *testing.T) {
	db, cleanup := createDB(t)
	defer cleanup()

	ctx := context.Background()

	u := UniqueNameUser{Name: "John", ID: 10}
	err := db.Save(ctx, &u)
	require.NoError(t, err)

	v := UniqueNameUser{}
	err = db.One(ctx, "Name", "John", &v)
	require.NoError(t, err)
	require.Equal(t, u, v)

	for i := 0; i < 10; i++ {
		w := IndexedNameUser{Name: "John", ID: i + 1, Group: "staff"}
		err = db.Save(ctx, &w)
		require.NoError(t, err)
	}

	var x IndexedNameUser
	err = db.One(ctx, "Name", "John", &x)
	require.NoError(t, err)
	require.Equal(t, "John", x.Name)
	require.Equal(t, 1, x.ID)
	require.Zero(t, x.age)
	require.True(t, x.DateOfBirth.IsZero())

	err = db.One(ctx, "Name", "Mike", &x)
	require.Error(t, err)
	require.ErrorIs(t, err, ErrNotFound)

	err = db.One(ctx, "", nil, &x)
	require.Error(t, err)
	require.ErrorIs(t, err, ErrNotFound)

	err = db.One(ctx, "", "Mike", nil)
	require.Error(t, err)
	require.ErrorIs(t, err, ErrStructPtrNeeded)

	err = db.One(ctx, "", nil, nil)
	require.Error(t, err)
	require.ErrorIs(t, err, ErrStructPtrNeeded)

	err = db.One(ctx, "Group", "staff", &x)
	require.NoError(t, err)
	require.Equal(t, 1, x.ID)

	err = db.One(ctx, "Score", 5, &x)
	require.NoError(t, err)
	require.Equal(t, 5, x.ID)

	err = db.One(ctx, "Group", "admin", &x)
	require.Error(t, err)
	require.ErrorIs(t, err, ErrNotFound)

	y := UniqueNameUser{Name: "Jake", ID: 200}
	err = db.Save(ctx, &y)
	require.NoError(t, err)

	var y2 UniqueNameUser
	err = db.One(ctx, "ID", 200, &y2)
	require.NoError(t, err)
	require.Equal(t, y, y2)

	n := NestedID{}
	n.ID = "100"
	n.Name = "John"

	err = db.Save(ctx, &n)
	require.NoError(t, err)

	var n2 NestedID
	err = db.One(ctx, "ID", "100", &n2)
	require.NoError(t, err)
	require.Equal(t, n, n2)
}

func TestOneNotWritable(t *testing.T) {
	dir, _ := os.MkdirTemp(os.TempDir(), "rainstorm")
	defer os.RemoveAll(dir)
	ctx := context.Background()
	db, _ := Open(ctx, filepath.Join(dir, "rainstorm.db"))

	err := db.Save(ctx, &User{ID: 10, Name: "John"})
	require.NoError(t, err)

	db.Close()

	db, _ = Open(ctx, filepath.Join(dir, "rainstorm.db"), BoltOptions(0660, &bolt.Options{
		ReadOnly: true,
	}))
	defer db.Close()

	err = db.Save(ctx, &User{ID: 20, Name: "John"})
	require.Error(t, err)

	var u User
	err = db.One(ctx, "ID", 10, &u)
	require.NoError(t, err)
	require.Equal(t, 10, u.ID)
	require.Equal(t, "John", u.Name)

	err = db.One(ctx, "Name", "John", &u)
	require.NoError(t, err)
	require.Equal(t, 10, u.ID)
	require.Equal(t, "John", u.Name)
}

func TestRange(t *testing.T) {
	db, cleanup := createDB(t)
	defer cleanup()

	ctx := context.Background()

	for i := 0; i < 100; i++ {
		w := User{
			Name:        "John",
			ID:          i + 1,
			Slug:        fmt.Sprintf("John%03d", i+1),
			DateOfBirth: time.Now().Add(-time.Duration(i) * time.Hour),
			Group:       fmt.Sprintf("Group%03d", i%5),
		}
		err := db.Save(ctx, &w)
		require.NoError(t, err)
		z := User{Name: fmt.Sprintf("Zach%03d", i+1), ID: i + 101, Slug: fmt.Sprintf("Zach%03d", i+1)}
		err = db.Save(ctx, &z)
		require.NoError(t, err)
	}

	min := "John010"
	max := "John020"
	var users []User

	err := db.Range(ctx, "Slug", min, max, users)
	require.ErrorIs(t, err, ErrSlicePtrNeeded)

	err = db.Range(ctx, "Slug", min, max, &users)
	require.NoError(t, err)
	require.Len(t, users, 11)
	require.Equal(t, "John010", users[0].Slug)
	require.Equal(t, "John020", users[10].Slug)

	err = db.Range(ctx, "Slug", min, max, &users, Reverse())
	require.NoError(t, err)
	require.Len(t, users, 11)
	require.Equal(t, "John020", users[0].Slug)
	require.Equal(t, "John010", users[10].Slug)

	min = "Zach010"
	max = "Zach020"
	users = nil
	err = db.Range(ctx, "Name", min, max, &users)
	require.NoError(t, err)
	require.Len(t, users, 11)
	require.Equal(t, "Zach010", users[0].Name)
	require.Equal(t, "Zach020", users[10].Name)

	err = db.Range(ctx, "Name", min, max, &users, Reverse())
	require.NoError(t, err)
	require.Len(t, users, 11)
	require.Equal(t, "Zach020", users[0].Name)
	require.Equal(t, "Zach010", users[10].Name)

	err = db.Range(ctx, "Name", min, max, &User{})
	require.Error(t, err)
	require.ErrorIs(t, err, ErrSlicePtrNeeded)

	notTheRightUsers := []UniqueNameUser{}

	err = db.Range(ctx, "Name", min, max, &notTheRightUsers)
	require.NoError(t, err)
	require.Equal(t, 0, len(notTheRightUsers))

	users = nil

	err = db.Range(ctx, "Age", min, max, &users)
	require.Error(t, err)
	require.EqualError(t, err, "not found")

	err = db.Range(ctx, "Age", 2, 5, &users)
	require.NoError(t, err)
	require.Len(t, users, 4)

	dateMin := time.Now().Add(-time.Duration(50) * time.Hour)
	dateMax := dateMin.Add(time.Duration(3) * time.Hour)
	err = db.Range(ctx, "DateOfBirth", dateMin, dateMax, &users)
	require.NoError(t, err)
	require.Len(t, users, 3)
	require.Equal(t, "John050", users[0].Slug)
	require.Equal(t, "John048", users[2].Slug)

	err = db.Range(ctx, "Slug", "John010", "John040", &users, Limit(10), Skip(20))
	require.NoError(t, err)
	require.Len(t, users, 10)
	require.Equal(t, 30, users[0].ID)
	require.Equal(t, 39, users[9].ID)

	err = db.Range(ctx, "Slug", "John010", "John040", &users, Limit(10), Skip(20), Reverse())
	require.NoError(t, err)
	require.Len(t, users, 10)
	require.Equal(t, 20, users[0].ID)
	require.Equal(t, 11, users[9].ID)

	err = db.Range(ctx, "Group", "Group002", "Group004", &users)
	require.NoError(t, err)
	require.Len(t, users, 60)
}

func TestPrefix(t *testing.T) {
	db, cleanup := createDB(t)
	defer cleanup()

	ctx := context.Background()

	for i := 0; i < 50; i++ {
		w := User{
			ID: i + 1,
		}

		if i%5 == 0 {
			w.Name = "John"
			w.Group = "group100"
		} else {
			w.Name = "Jack"
			w.Group = "group200"
		}

		err := db.Save(ctx, &w)
		require.NoError(t, err)
	}

	var users []User
	err := db.Prefix(ctx, "Name", "Jo", users)
	require.ErrorIs(t, err, ErrSlicePtrNeeded)

	// Using indexes
	err = db.Prefix(ctx, "Name", "Jo", &users)
	require.NoError(t, err)
	require.Len(t, users, 10)
	require.Equal(t, 1, users[0].ID)
	require.Equal(t, 46, users[9].ID)

	err = db.Prefix(ctx, "Name", "Ja", &users)
	require.NoError(t, err)
	require.Len(t, users, 40)
	require.Equal(t, 2, users[0].ID)
	require.Equal(t, 50, users[39].ID)

	err = db.Prefix(ctx, "Name", "Ja", &users, Limit(10), Skip(20), Reverse())
	require.NoError(t, err)
	require.Len(t, users, 10)
	require.Equal(t, 25, users[0].ID)
	require.Equal(t, 14, users[9].ID)

	// Using Select
	err = db.Prefix(ctx, "Group", "group1", &users)
	require.NoError(t, err)
	require.Len(t, users, 10)
	require.Equal(t, 1, users[0].ID)
	require.Equal(t, 46, users[9].ID)

	err = db.Prefix(ctx, "Group", "group2", &users)
	require.NoError(t, err)
	require.Len(t, users, 40)
	require.Equal(t, 2, users[0].ID)
	require.Equal(t, 50, users[39].ID)

	err = db.Prefix(ctx, "Group", "group2", &users, Limit(10), Skip(20), Reverse())
	require.NoError(t, err)
	require.Len(t, users, 10)
	require.Equal(t, 25, users[0].ID)
	require.Equal(t, 14, users[9].ID)

	// Bad value
	err = db.Prefix(ctx, "Group", "group3", &users)
	require.ErrorIs(t, err, ErrNotFound)
}

func TestPrefixWithID(t *testing.T) {
	db, cleanup := createDB(t)
	defer cleanup()

	ctx := context.Background()

	type User struct {
		ID string
	}

	require.NoError(t, db.Save(ctx, &User{ID: "1"}))
	require.NoError(t, db.Save(ctx, &User{ID: "10"}))

	var users []User

	require.NoError(t, db.Prefix(ctx, "ID", "1", &users))
	require.Len(t, users, 2)
}
