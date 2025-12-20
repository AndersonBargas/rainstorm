package aes

import (
	"encoding/base64"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/AndersonBargas/rainstorm/v6/codec/internal"
	"github.com/AndersonBargas/rainstorm/v6/codec/json"
)

var testKey, _ = base64.StdEncoding.DecodeString("xkBTXc1wn0C/aL31u9SA7g==")

func TestAES(t *testing.T) {
	aes, err := NewAES(json.Codec, testKey)
	require.NoError(t, err)

	internal.RoundtripTester(t, aes)
}

func TestName(t *testing.T) {
	c, err := NewAES(json.Codec, testKey)
	require.NoError(t, err)
	require.Equal(t, "aes-json", c.Name())
}

func TestAESErrors(t *testing.T) {
	// Invalid key size
	_, err := NewAES(json.Codec, []byte("short"))
	require.Error(t, err)

	c, _ := NewAES(json.Codec, testKey)

	// Unmarshal invalid data (too short)
	err = c.Unmarshal([]byte("short"), nil)
	require.Error(t, err)
}
