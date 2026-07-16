package rainstorm

import (
	"io"
	"time"
)

type ClassicNoTags struct {
	PublicField int
	//lint:ignore U1000 Private fields are deliberate reflection fixtures.
	privateField string
	Date         time.Time
	InlineStruct struct {
		a float32
		B float64
	}
	Interf io.Writer
}

type ClassicBadTags struct {
	ID          string
	PublicField int `rainstorm:"mrots"`
	//lint:ignore U1000 Private fields are deliberate reflection fixtures.
	privateField string
	Date         time.Time
	InlineStruct struct {
		a float32
		B float64
	}
	Interf io.Writer
}

type ClassicUnique struct {
	ID          string
	PublicField int `rainstorm:"unique"`
	//lint:ignore U1000 Private tagged fields verify reflection visibility rules.
	privateField string `rainstorm:"unique"`
	//lint:ignore U1000 Private tagged fields verify reflection visibility rules.
	privateField2 string    `rainstorm:"unique"`
	Date          time.Time `rainstorm:"unique"`
	InlineStruct  struct {
		A float32
		B float64
	} `rainstorm:"unique"`
	Interf io.Writer `rainstorm:"unique"`
}

type ClassicIndex struct {
	ID          string
	PublicField int `rainstorm:"index"`
	//lint:ignore U1000 Private tagged fields verify reflection visibility rules.
	privateField string    `rainstorm:"index"`
	Date         time.Time `rainstorm:"index"`
	InlineStruct struct {
		a float32
		B float64
	} `rainstorm:"index"`
	InlineStructPtr *UserWithNoID `rainstorm:"index"`
	Interf          io.Writer     `rainstorm:"index"`
}

type ClassicInline struct {
	PublicField  int `rainstorm:"unique"`
	ClassicIndex `rainstorm:"inline"`
	*ToEmbed     `rainstorm:"inline"`
	Date         time.Time `rainstorm:"unique"`
}

type User struct {
	ID          int       `rainstorm:"id,increment"`
	Name        string    `rainstorm:"index"`
	Age         int       `rainstorm:"index,increment"`
	DateOfBirth time.Time `rainstorm:"index"`
	Group       string
	//lint:ignore U1000 Private fields are deliberate reflection fixtures.
	unexportedField int
	Slug            string `rainstorm:"unique"`
}

type ToEmbed struct {
	ID string
}

type NestedID struct {
	ToEmbed `rainstorm:"inline"`
	Name    string
}

type SimpleUser struct {
	ID   int `rainstorm:"id"`
	Name string
	age  int
}

type UserWithNoID struct {
	Name string
}

type UserWithIDField struct {
	ID   int
	Name string
}

type UserWithUint64IDField struct {
	ID   uint64
	Name string
}

type UserWithStringIDField struct {
	ID   string
	Name string
}

type UserWithEmbeddedIDField struct {
	UserWithIDField `rainstorm:"inline"`
	Age             int
}

type UserWithEmbeddedField struct {
	UserWithNoID `rainstorm:"inline"`
	ID           uint64
}

type UserWithIncrementField struct {
	ID   int
	Name string
	Age  int `rainstorm:"unique,increment"`
}

type IndexedNameUser struct {
	ID          int    `rainstorm:"id"`
	Name        string `rainstorm:"index"`
	Score       int    `rainstorm:"index,increment"`
	age         int
	DateOfBirth time.Time `rainstorm:"index"`
	Group       string
}

type UniqueNameUser struct {
	ID   int    `rainstorm:"id"`
	Name string `rainstorm:"unique"`
	Age  int    `rainstorm:"index,increment"`
}
