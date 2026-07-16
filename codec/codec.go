// Package codec contains sub-packages with different codecs that can be used
// to encode and decode entities in Rainstorm.
package codec

// MarshalUnmarshaler represents a codec used to marshal and unmarshal entities.
type MarshalUnmarshaler interface {
	// Marshal encodes v.
	Marshal(v interface{}) ([]byte, error)
	// Unmarshal decodes b into v.
	Unmarshal(b []byte, v interface{}) error
	// Name returns the stable codec identifier stored in Rainstorm metadata.
	Name() string
}
