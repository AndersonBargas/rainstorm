package rainstorm

import (
	"reflect"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestSorterCompareValue(t *testing.T) {
	s := &sorter{}

	// Int
	require.Equal(t, -1, s.compareValue(reflect.ValueOf(1), reflect.ValueOf(2)))
	require.Equal(t, 1, s.compareValue(reflect.ValueOf(2), reflect.ValueOf(1)))
	require.Equal(t, 0, s.compareValue(reflect.ValueOf(1), reflect.ValueOf(1)))

	// Uint
	require.Equal(t, -1, s.compareValue(reflect.ValueOf(uint(1)), reflect.ValueOf(uint(2))))
	require.Equal(t, 1, s.compareValue(reflect.ValueOf(uint(2)), reflect.ValueOf(uint(1))))
	require.Equal(t, 0, s.compareValue(reflect.ValueOf(uint(1)), reflect.ValueOf(uint(1))))

	// Float
	require.Equal(t, -1, s.compareValue(reflect.ValueOf(1.1), reflect.ValueOf(2.2)))
	require.Equal(t, 1, s.compareValue(reflect.ValueOf(2.2), reflect.ValueOf(1.1)))
	require.Equal(t, 0, s.compareValue(reflect.ValueOf(1.1), reflect.ValueOf(1.1)))

	// String
	require.Equal(t, -1, s.compareValue(reflect.ValueOf("a"), reflect.ValueOf("b")))
	require.Equal(t, 1, s.compareValue(reflect.ValueOf("b"), reflect.ValueOf("a")))
	require.Equal(t, 0, s.compareValue(reflect.ValueOf("a"), reflect.ValueOf("a")))

	// Time
	now := time.Now()
	after := now.Add(time.Hour)
	require.Equal(t, -1, s.compareValue(reflect.ValueOf(now), reflect.ValueOf(after)))
	require.Equal(t, 1, s.compareValue(reflect.ValueOf(after), reflect.ValueOf(now)))
	require.Equal(t, 0, s.compareValue(reflect.ValueOf(now), reflect.ValueOf(now)))

	// Invalid
	var invalid reflect.Value
	require.Equal(t, -1, s.compareValue(invalid, reflect.ValueOf(1)))
	require.Equal(t, 1, s.compareValue(reflect.ValueOf(1), invalid))
	require.Equal(t, -1, s.compareValue(invalid, invalid))
}
