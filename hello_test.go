package main

import (
	"bytes"
	"fmt"
	"testing"

	"github.com/ipfs/go-cid"
)

func TestRoundTripMessage(t *testing.T) {
	a, _ := cid.Decode("zAe7WNEQGHoHwYU7eLihfiZAi45dFZCZuVex1HX7fs9kaebt1MTJ")
	b, _ := cid.Decode("zAe7WNEQPqiWQYohotrjBVvmtfaeuuziQ9crYv3Bq7oCEL7DY8Cx")
	in := HelloMessage{
		HeaviestTipSet: []cid.Cid{a},
		GenesisHash:    b,
	}

	buf := new(bytes.Buffer)

	if err := WriteCborRPC(buf, &in); err != nil {
		t.Fatal(err)
	}

	var out HelloMessage

	if err := ReadCborRPC(buf, &out); err != nil {
		t.Fatal(err)
	}

	fmt.Println(in)
	fmt.Println(out)
}
