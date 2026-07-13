package rainstorm

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"time"

	"github.com/AndersonBargas/rainstorm/v6/codec"
	"github.com/AndersonBargas/rainstorm/v6/codec/json"
	bolt "go.etcd.io/bbolt"
)

const (
	dbinfo         = "__rainstorm_db"
	metadataBucket = "__rainstorm_metadata"
)

// BucketNamer is an interface that can be implemented by structs to provide
// a custom bucket name. This is essential for runtime-generated types
// (via reflect.StructOf) that have no static type name.
type BucketNamer interface {
	RainstormBucketName() string
}

// Defaults to json
var defaultCodec = json.Codec

// Open opens a database at the given path with optional Rainstorm options.
func Open(ctx context.Context, path string, rainstormOptions ...OpenOption) (*DB, error) {
	if err := checkContext(ctx); err != nil {
		return nil, err
	}

	var err error

	var opts Options
	for _, option := range rainstormOptions {
		if err = option(&opts); err != nil {
			return nil, err
		}
	}

	s := DB{
		bolt: opts.bolt,
	}

	n := node{
		s:          &s,
		codec:      opts.codec,
		rootBucket: opts.rootBucket,
	}

	if n.codec == nil {
		n.codec = defaultCodec
	}

	if opts.boltMode == 0 {
		opts.boltMode = 0600
	}

	if opts.boltOptions == nil {
		opts.boltOptions = &bolt.Options{Timeout: 1 * time.Second}
	}

	s.Node = &n

	// skip if UseDB option is used
	if s.bolt == nil {
		s.bolt, err = bolt.Open(path, opts.boltMode, opts.boltOptions)
		if err != nil {
			return nil, err
		}
		s.boltOwned = true
		if opts.postOpenHook != nil {
			opts.postOpenHook(ctx)
		}
	}

	// Re-check the context after the (cooperative) bbolt open.
	if err = checkContext(ctx); err != nil {
		s.cleanupOwned()
		return nil, err
	}

	err = s.checkVersion(ctx)
	if err != nil {
		s.cleanupOwned()
		return nil, err
	}

	return &s, nil
}

// DB is the wrapper around BoltDB. It contains an instance of BoltDB and uses it to perform all the
// needed operations
type DB struct {
	// The root node that points to the root bucket.
	Node

	// bolt is the underlying bbolt database.
	bolt *bolt.DB

	// boltOwned records whether Rainstorm opened Bolt itself (true) or whether
	// it was provided via UseDB (false).
	boltOwned bool
}

// NativeDB returns the underlying bbolt database.
//
// This is an advanced interoperability escape hatch. Native operations bypass
// Rainstorm context checkpoints. Native writes can bypass codecs, indexes,
// metadata, and invariants. Rainstorm cannot guarantee cancellation, rollback
// composition, index consistency, or destination safety for native operations.
// Callers must not close the returned database while Rainstorm is in use.
// Callers are responsible for coordinating native transactions with Rainstorm
// operations. Normal application code should prefer Rainstorm APIs and managed
// transactions.
func (db *DB) NativeDB() *bolt.DB {
	if db == nil {
		return nil
	}
	return db.bolt
}

// cleanupOwned closes a Rainstorm-owned bolt.DB. It is a no-op for databases
// provided via UseDB, which remain owned by the caller.
func (s *DB) cleanupOwned() {
	if s.boltOwned && s.bolt != nil {
		_ = s.bolt.Close()
	}
}

// Close the database.
//
// For a Rainstorm-owned database (opened via Open without UseDB), Close closes
// the underlying bbolt database and returns its error.
//
// For a borrowed database (opened via UseDB), Close returns nil and does not
// close the underlying bbolt database. The caller retains ownership.
//
// If the receiver is nil or the underlying bbolt database is nil, Close returns
// ErrNilParam without panicking.
func (s *DB) Close() error {
	if s == nil {
		return ErrNilParam
	}
	if s.bolt == nil {
		return ErrNilParam
	}
	if !s.boltOwned {
		return nil
	}
	return s.bolt.Close()
}

func (s *DB) checkVersion(ctx context.Context) error {
	var v string
	err := s.Get(ctx, dbinfo, "version", &v)
	if err != nil && !errors.Is(err, ErrNotFound) {
		return err
	}

	// for now, we only set the current version if it doesn't exist.
	// v1 and v2 database files are compatible.
	if v == "" {
		return s.Set(ctx, dbinfo, "version", Version)
	}

	return nil
}

// toBytes turns an interface into a slice of bytes
func toBytes(key interface{}, codec codec.MarshalUnmarshaler) ([]byte, error) {
	if key == nil {
		return nil, nil
	}
	switch t := key.(type) {
	case []byte:
		return t, nil
	case string:
		return []byte(t), nil
	case int:
		return numbertob(int64(t))
	case uint:
		return numbertob(uint64(t))
	case int8, int16, int32, int64, uint8, uint16, uint32, uint64:
		return numbertob(t)
	default:
		return codec.Marshal(key)
	}
}

func numbertob(v interface{}) ([]byte, error) {
	var buf bytes.Buffer
	err := binary.Write(&buf, binary.BigEndian, v)
	if err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func numberfromb(raw []byte) (int64, error) {
	r := bytes.NewReader(raw)
	var to int64
	err := binary.Read(r, binary.BigEndian, &to)
	if err != nil {
		return 0, err
	}
	return to, nil
}
