package codec_test

import (
	"fmt"

	"github.com/AndersonBargas/rainstorm/v6"
	"github.com/AndersonBargas/rainstorm/v6/codec/gob"
	"github.com/AndersonBargas/rainstorm/v6/codec/json"
	"github.com/AndersonBargas/rainstorm/v6/codec/msgpack"
	"github.com/AndersonBargas/rainstorm/v6/codec/protobuf"
	"github.com/AndersonBargas/rainstorm/v6/codec/sereal"
	"github.com/AndersonBargas/rainstorm/v6/internal/testadaptor"
)

func Example() {
	// The examples below show how to set up all the codecs shipped with Rainstorm.
	// Proper error handling left out to make it simple.
	var gobBDB, _ = testadaptor.Open("gob.db", 0600, nil)
	var gobDb, _ = rainstorm.New(gobBDB, rainstorm.Codec(gob.Codec))
	var jsonBDB, _ = testadaptor.Open("json.db", 0600, nil)
	var jsonDb, _ = rainstorm.New(jsonBDB, rainstorm.Codec(json.Codec))
	var msgpackBDB, _ = testadaptor.Open("msgpack.db", 0600, nil)
	var msgpackDb, _ = rainstorm.New(msgpackBDB, rainstorm.Codec(msgpack.Codec))
	var serealBDB, _ = testadaptor.Open("sereal.db", 0600, nil)
	var serealDb, _ = rainstorm.New(serealBDB, rainstorm.Codec(sereal.Codec))
	var protobufBDB, _ = testadaptor.Open("protobuf.db", 0600, nil)
	var protobufDb, _ = rainstorm.New(protobufBDB, rainstorm.Codec(protobuf.Codec))

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
