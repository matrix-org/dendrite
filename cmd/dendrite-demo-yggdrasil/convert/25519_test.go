// Copyright 2020 The Matrix.org Foundation C.I.C.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

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
