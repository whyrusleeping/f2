package main

import (
	//"encoding/binary"
	//"fmt"
	"io"

	cbor "github.com/ipfs/go-ipld-cbor"
	//refmt "github.com/polydawn/refmt"
)

const MessageSizeLimit = 1 << 20

func WriteCborRPC(w io.Writer, obj interface{}) error {
	data, err := cbor.DumpObject(obj)
	if err != nil {
		return err
	}

	/*
		vbuf := make([]byte, 8)
		n := binary.PutUvarint(vbuf, uint64(len(data)))

		_, err = w.Write(vbuf[:n])
		if err != nil {
			return err
		}
	*/

	_, err = w.Write(data)
	return err
}

type ByteReader interface {
	io.Reader
	io.ByteReader
}

func ReadCborRPC(r ByteReader, out interface{}) error {
	return cbor.DecodeReader(r, out)
}
