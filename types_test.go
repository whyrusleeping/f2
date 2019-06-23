package main

import (
	"bytes"
	"encoding/hex"
	"fmt"
	"testing"

	cbor "github.com/ipfs/go-ipld-cbor"
)

var testMessage = "d82c885501fd1d0f4dfcd7e99afcb99a8326b7dc459d32c6285501b882619d46558f3d9e316d11b48dcf211327026a1875c245037e11d600c241c8c2430dbba0666d6574686f644d706172616d73617265676f6f64"

var testBlock = "d82b895501fd1d0f4dfcd7e99afcb99a8326b7dc459d32c628814a69616d617469636b6574566920616d20616e20656c656374696f6e2070726f6f6681d82a5827000171a0e40220ce25e43084e66e5a92f8c3066c00c0eb540ac2f2a173326507908da06b96f678c242bb6a1a0012d687d82a5827000171a0e40220ce25e43084e66e5a92f8c3066c00c0eb540ac2f2a173326507908da06b96f6788080"

func TestMessageRoundTrip(t *testing.T) {
	b, err := hex.DecodeString(testMessage)
	if err != nil {
		t.Fatal(err)
	}

	msg, err := DecodeMessage(b)
	if err != nil {
		t.Fatal(err)
	}

	out, err := msg.Serialize()
	if err != nil {
		panic(err)
	}

	if !bytes.Equal(b, out) {
		t.Fatal("input and output do not match")
	}
}

func TestSignedMessageRoundTrip(t *testing.T) {
	smsg := &SignedMessage{
		Message: Message{},
		Signature: Signature{
			Type: "secp256k1",
			Data: []byte("cats"),
		},
	}

	data, err := cbor.DumpObject(smsg)
	if err != nil {
		t.Fatal(err)
	}

	var osig SignedMessage
	if err := cbor.DecodeInto(data, &osig); err != nil {
		t.Fatal(err)
	}

}

func TestSignatureRoundTrip(t *testing.T) {
	sig := &Signature{
		Type: "secp256k1",
		Data: []byte("cats"),
	}

	data, err := cbor.DumpObject(sig)
	if err != nil {
		t.Fatal(err)
	}

	fmt.Println(data)

	var osig Signature
	if err := cbor.DecodeInto(data, &osig); err != nil {
		t.Fatal(err)
	}

}

func TestBlockRoundTrip(t *testing.T) {
	b, err := hex.DecodeString(testBlock)
	if err != nil {
		t.Fatal(err)
	}

	blk, err := DecodeBlock(b)
	if err != nil {
		t.Fatal(err)
	}

	fmt.Printf("%#v\n", blk)

	out, err := blk.Serialize()
	if err != nil {
		t.Fatal(err)
	}

	if !bytes.Equal(b, out) {
		t.Fatal("input and output do not match")
	}
}

/*
func TestMissingFieldRoundTrip(t *testing.T) {
		//a, _ := cid.Decode("zAe7WNEQPqiWQYohotrjBVvmtfaeuuziQ9crYv3Bq7oCEL7DY8Cx")
		//b := &Block{
			//StateRoot: a,
			//Tickets:   []Ticket{},
			//Parents:   []cid.Cid{a},
		//}
	cst := hamt.NewCborStore()
	b, _ := MakeGenesisBlock(cst)
	b.Messages = []SignedMessage{
		SignedMessage{
			Message: {},
			Signature: {
				Type: 1,
				Data: []byte("cats"),
			},
		},
	}

	before := b.Cid()

	data, err := b.Serialize()
	if err != nil {
		t.Fatal(err)
	}

	out, err := DecodeBlock(data)
	if err != nil {
		t.Fatal(err)
	}

	if before != out.Cid() {
		fmt.Printf("%#v\n", b)
		fmt.Printf("%#v\n", out)
		t.Fatal("round trip failed")
	}

}
*/
