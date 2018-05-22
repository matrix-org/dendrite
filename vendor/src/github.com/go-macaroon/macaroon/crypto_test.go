package macaroon

import (
	"crypto/rand"
	"fmt"

	"golang.org/x/crypto/nacl/secretbox"
	gc "gopkg.in/check.v1"
)

type cryptoSuite struct{}

var _ = gc.Suite(&cryptoSuite{})

var testCryptKey = &[hashLen]byte{'k', 'e', 'y'}
var testCryptText = &[hashLen]byte{'t', 'e', 'x', 't'}

func (*cryptoSuite) TestEncDec(c *gc.C) {
	b, err := encrypt(testCryptKey, testCryptText, rand.Reader)
	c.Assert(err, gc.IsNil)
	t, err := decrypt(testCryptKey, b)
	c.Assert(err, gc.IsNil)
	c.Assert(string(t[:]), gc.Equals, string(testCryptText[:]))
}

func (*cryptoSuite) TestUniqueNonces(c *gc.C) {
	nonces := make(map[string]struct{})
	for i := 0; i < 100; i++ {
		nonce, err := newNonce(rand.Reader)
		c.Assert(err, gc.IsNil)
		nonces[string(nonce[:])] = struct{}{}
	}
	c.Assert(nonces, gc.HasLen, 100, gc.Commentf("duplicate nonce detected"))
}

type ErrorReader struct{}

func (*ErrorReader) Read([]byte) (int, error) {
	return 0, fmt.Errorf("fail")
}

func (*cryptoSuite) TestBadRandom(c *gc.C) {
	_, err := newNonce(&ErrorReader{})
	c.Assert(err, gc.ErrorMatches, "^cannot generate random bytes:.*")

	_, err = encrypt(testCryptKey, testCryptText, &ErrorReader{})
	c.Assert(err, gc.ErrorMatches, "^cannot generate random bytes:.*")
}

func (*cryptoSuite) TestBadCiphertext(c *gc.C) {
	buf := randomBytes(nonceLen + secretbox.Overhead)
	for i := range buf {
		_, err := decrypt(testCryptKey, buf[0:i])
		c.Assert(err, gc.ErrorMatches, "message too short")
	}
	_, err := decrypt(testCryptKey, buf)
	c.Assert(err, gc.ErrorMatches, "decryption failure")
}

func randomBytes(n int) []byte {
	buf := make([]byte, n)
	if _, err := rand.Reader.Read(buf); err != nil {
		panic(err)
	}
	return buf
}
