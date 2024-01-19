package main

import (
	"bytes"
	"crypto/rand"
	"io"
	"log"

	cosmtypes "github.com/cosmos/cosmos-sdk/types"
)

const defaultKeyringKeyName = "validator"

var emptyCosmosAddress = cosmtypes.AccAddress{}

func newPassReader(pass string) io.Reader {
	return &passReader{
		pass: pass,
		buf:  new(bytes.Buffer),
	}
}

type passReader struct {
	pass string
	buf  *bytes.Buffer
}

var _ io.Reader = &passReader{}

func (r *passReader) Read(p []byte) (n int, err error) {
	n, err = r.buf.Read(p)
	if err == io.EOF || n == 0 {
		r.buf.WriteString(r.pass + "\n")

		n, err = r.buf.Read(p)
	}

	return
}

func randPhrase(size int) string {
	buf := make([]byte, size)
	_, err := rand.Read(buf)
	orPanic(err)

	return string(buf)
}

func orPanic(err error) {
	if err != nil {
		log.Panicln()
	}
}
