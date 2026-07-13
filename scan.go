package rainstorm

import (
	"bytes"
	"context"

	bolt "go.etcd.io/bbolt"
)

// A BucketScanner scans a Node for a list of buckets
type BucketScanner interface {
	// PrefixScan scans the root buckets for keys matching the given prefix.
	PrefixScan(ctx context.Context, prefix string) ([]Node, error)
	// RangeScan scans the buckets in this node for keys matching the given range.
	RangeScan(ctx context.Context, min, max string) ([]Node, error)
}

// PrefixScan scans the buckets in this node for keys matching the given prefix.
func (n *node) PrefixScan(ctx context.Context, prefix string) ([]Node, error) {
	if err := checkContext(ctx); err != nil {
		return nil, err
	}

	if n.tx != nil {
		return n.prefixScan(ctx, n.tx, prefix)
	}

	var nodes []Node
	err := n.readTx(ctx, func(tx *bolt.Tx) error {
		if err := checkContext(ctx); err != nil {
			return err
		}
		var scanErr error
		nodes, scanErr = n.prefixScan(ctx, tx, prefix)
		return scanErr
	})
	if err != nil {
		return nil, err
	}
	return nodes, nil
}

func (n *node) prefixScan(ctx context.Context, tx *bolt.Tx, prefix string) ([]Node, error) {
	if err := checkContext(ctx); err != nil {
		return nil, err
	}

	prefixBytes := []byte(prefix)
	var nodes []Node

	if err := checkContext(ctx); err != nil {
		return nil, err
	}
	c := n.cursor(tx)
	if err := checkContext(ctx); err != nil {
		return nil, err
	}

	if c == nil {
		return nil, nil
	}

	for k, v := c.Seek(prefixBytes); k != nil && bytes.HasPrefix(k, prefixBytes); k, v = c.Next() {
		if err := checkContext(ctx); err != nil {
			return nil, err
		}
		if v != nil {
			continue
		}

		if err := checkContext(ctx); err != nil {
			return nil, err
		}
		node := n.From(string(k))
		if err := checkContext(ctx); err != nil {
			return nil, err
		}
		nodes = append(nodes, node)
	}

	if err := checkContext(ctx); err != nil {
		return nil, err
	}

	return nodes, nil
}

// RangeScan scans the buckets in this node  over a range such as a sortable time range.
func (n *node) RangeScan(ctx context.Context, min, max string) ([]Node, error) {
	if err := checkContext(ctx); err != nil {
		return nil, err
	}

	if n.tx != nil {
		return n.rangeScan(ctx, n.tx, min, max)
	}

	var nodes []Node
	err := n.readTx(ctx, func(tx *bolt.Tx) error {
		if err := checkContext(ctx); err != nil {
			return err
		}
		var scanErr error
		nodes, scanErr = n.rangeScan(ctx, tx, min, max)
		return scanErr
	})
	if err != nil {
		return nil, err
	}
	return nodes, nil
}

func (n *node) rangeScan(ctx context.Context, tx *bolt.Tx, min, max string) ([]Node, error) {
	if err := checkContext(ctx); err != nil {
		return nil, err
	}

	minBytes := []byte(min)
	maxBytes := []byte(max)
	var nodes []Node

	if err := checkContext(ctx); err != nil {
		return nil, err
	}
	c := n.cursor(tx)
	if err := checkContext(ctx); err != nil {
		return nil, err
	}
	if c == nil {
		return nil, nil
	}

	for k, v := c.Seek(minBytes); k != nil && bytes.Compare(k, maxBytes) <= 0; k, v = c.Next() {
		if err := checkContext(ctx); err != nil {
			return nil, err
		}
		if v != nil {
			continue
		}

		if err := checkContext(ctx); err != nil {
			return nil, err
		}
		node := n.From(string(k))
		if err := checkContext(ctx); err != nil {
			return nil, err
		}
		nodes = append(nodes, node)
	}

	if err := checkContext(ctx); err != nil {
		return nil, err
	}

	return nodes, nil
}

func (n *node) cursor(tx *bolt.Tx) *bolt.Cursor {
	var c *bolt.Cursor

	if len(n.rootBucket) > 0 {
		b := n.GetBucket(tx)
		if b == nil {
			return nil
		}
		c = b.Cursor()
	} else {
		c = tx.Cursor()
	}

	return c
}
