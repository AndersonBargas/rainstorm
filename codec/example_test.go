package codec_test

import (
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"

	"github.com/AndersonBargas/rainstorm/v6"
	"github.com/AndersonBargas/rainstorm/v6/codec"
	"github.com/AndersonBargas/rainstorm/v6/codec/gob"
	"github.com/AndersonBargas/rainstorm/v6/codec/json"
	"github.com/AndersonBargas/rainstorm/v6/codec/msgpack"
	"github.com/AndersonBargas/rainstorm/v6/codec/protobuf"
	"github.com/AndersonBargas/rainstorm/v6/codec/sereal"
)

func Example() {
	ctx := context.Background()
	dir, err := os.MkdirTemp("", "rainstorm-codecs")
	if err != nil {
		log.Fatal(err)
	}
	defer func() {
		if err := os.RemoveAll(dir); err != nil {
			log.Println("remove temporary directory:", err)
		}
	}()

	codecs := []struct {
		name  string
		codec codec.MarshalUnmarshaler
	}{
		{name: "gob", codec: gob.Codec},
		{name: "json", codec: json.Codec},
		{name: "msgpack", codec: msgpack.Codec},
		{name: "sereal", codec: sereal.Codec},
		{name: "protobuf", codec: protobuf.Codec},
	}

	for _, candidate := range codecs {
		db, err := rainstorm.Open(ctx, filepath.Join(dir, candidate.name+".db"), rainstorm.Codec(candidate.codec))
		if err != nil {
			log.Fatal(err)
		}

		fmt.Printf("%T\n", db.Codec())
		if err := db.Close(); err != nil {
			log.Fatal(err)
		}
	}

	// Output:
	// *gob.gobCodec
	// *json.jsonCodec
	// *msgpack.msgpackCodec
	// *sereal.serealCodec
	// *protobuf.protobufCodec
}
