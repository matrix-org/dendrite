package convert

import (
	"bytes"
	"crypto/ed25519"
	"encoding/hex"
	"testing"

	"golang.org/x/crypto/curve25519"
)

func TestKeyConversion(t *testing.T) {
	edPub, edPriv, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Log("Signing public:", hex.EncodeToString(edPub))
	t.Log("Signing private:", hex.EncodeToString(edPriv))

	cuPriv := Ed25519PrivateKeyToCurve25519(edPriv)
	t.Log("Encryption private:", hex.EncodeToString(cuPriv))

	cuPub := Ed25519PublicKeyToCurve25519(edPub)
	t.Log("Converted encryption public:", hex.EncodeToString(cuPub))

	var realPub, realPriv [32]byte
	copy(realPriv[:32], cuPriv[:32])
	curve25519.ScalarBaseMult(&realPub, &realPriv)
	t.Log("Scalar-multed encryption public:", hex.EncodeToString(realPub[:]))

	if !bytes.Equal(realPriv[:], cuPriv[:]) {
		t.Fatal("Private keys should be equal (this means the test is broken)")
	}
	if !bytes.Equal(realPub[:], cuPub[:]) {
		t.Fatal("Public keys should be equal")
	}
}
