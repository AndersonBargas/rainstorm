package rainstorm

import bolt "go.etcd.io/bbolt"

// CreateBucketIfNotExists creates the bucket below the current node if it doesn't
// already exist.
func (n *node) CreateBucketIfNotExists(tx *bolt.Tx, bucket string) (*bolt.Bucket, error) {
	var b *bolt.Bucket
	var err error

	bucketNames := n.rootBucket
	if bucket != "" {
		bucketNames = append(bucketNames, bucket)
	}

	for _, bucketName := range bucketNames {
		if bucketName == "" {
			continue
		}
		if b != nil {
			if b, err = b.CreateBucketIfNotExists([]byte(bucketName)); err != nil {
				return nil, err
			}

		} else {
			if b, err = tx.CreateBucketIfNotExists([]byte(bucketName)); err != nil {
				return nil, err
			}
		}
	}

	// If there were no valid bucket names at all, fall back to the tx root
	if b == nil {
		return nil, ErrNoName
	}

	return b, nil
}

// GetBucket returns the given bucket below the current node.
func (n *node) GetBucket(tx *bolt.Tx, children ...string) *bolt.Bucket {
	var b *bolt.Bucket

	bucketNames := n.rootBucket
	for _, child := range children {
		if child != "" {
			bucketNames = append(bucketNames, child)
		}
	}
	for _, bucketName := range bucketNames {
		if bucketName == "" {
			continue
		}
		if b != nil {
			if b = b.Bucket([]byte(bucketName)); b == nil {
				return nil
			}
		} else {
			if b = tx.Bucket([]byte(bucketName)); b == nil {
				return nil
			}
		}
	}

	return b
}
