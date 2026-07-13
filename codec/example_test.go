package codec_test

import (
	"context"
	"fmt"

	"github.com/AndersonBargas/rainstorm/v6"
	"github.com/AndersonBargas/rainstorm/v6/codec/gob"
	"github.com/AndersonBargas/rainstorm/v6/codec/json"
	"github.com/AndersonBargas/rainstorm/v6/codec/msgpack"
	"github.com/AndersonBargas/rainstorm/v6/codec/protobuf"
	"github.com/AndersonBargas/rainstorm/v6/codec/sereal"
)

func Example() {
	ctx := context.Background()
	// The examples below show how to set up all the codecs shipped with Rainstorm.
	// Proper error handling left out to make it simple.
	var gobDb, _ = rainstorm.Open(ctx, "gob.db", rainstorm.Codec(gob.Codec))
	var jsonDb, _ = rainstorm.Open(ctx, "json.db", rainstorm.Codec(json.Codec))
	var msgpackDb, _ = rainstorm.Open(ctx, "msgpack.db", rainstorm.Codec(msgpack.Codec))
	var serealDb, _ = rainstorm.Open(ctx, "sereal.db", rainstorm.Codec(sereal.Codec))
	var protobufDb, _ = rainstorm.Open(ctx, "protobuf.db", rainstorm.Codec(protobuf.Codec))

	fmt.Printf("%T\n", gobDb.Codec())
	fmt.Printf("%T\n", jsonDb.Codec())
	fmt.Printf("%T\n", msgpackDb.Codec())
	fmt.Printf("%T\n", serealDb.Codec())
	fmt.Printf("%T\n", protobufDb.Codec())

	// Output:
	// *gob.gobCodec
	// *json.jsonCodec
	// *msgpack.msgpackCodec
	// *sereal.serealCodec
	// *protobuf.protobufCodec
}
