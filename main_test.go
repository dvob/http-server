package main

import (
	"bytes"
	"io"
	"testing"
)

func TestNBytesReader_read0(t *testing.T) {
	r := newNBytesReader(0)
	n, err := io.Copy(io.Discard, r)
	if err != nil {
		t.Fatal(err)
	}
	if n != 0 {
		t.Fatalf("read >0 bytes")
	}
}

func TestNBytesReader_readN(t *testing.T) {
	r := newNBytesReader(1337)
	buffer := &bytes.Buffer{}
	n, err := io.Copy(buffer, r)
	if err != nil {
		t.Fatal(err)
	}
	if n != 1337 || len(buffer.Bytes()) != 1337 {
		t.Fatalf("bytes != 1337")
	}
}
