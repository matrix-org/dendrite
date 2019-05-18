/* Copyright 2016-2017 Vector Creations Ltd
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

package gomatrixserverlib

import (
	"encoding/json"
	"fmt"

	"golang.org/x/crypto/ed25519"
)

// A KeyID is the ID of a ed25519 key used to sign JSON.
// The key IDs have a format of "ed25519:[0-9A-Za-z]+"
// If we switch to using a different signing algorithm then we will change the
// prefix used.
type KeyID string

// SignJSON signs a JSON object returning a copy signed with the given key.
// https://matrix.org/docs/spec/server_server/unstable.html#signing-json
func SignJSON(signingName string, keyID KeyID, privateKey ed25519.PrivateKey, message []byte) ([]byte, error) {
	// Unpack the top-level key of the JSON object without unpacking the contents of the keys.
	// This allows us to add and remove the top-level keys from the JSON object.
	// It also ensures that the JSON is actually a valid JSON object.
	var object map[string]*json.RawMessage
	var signatures map[string]map[KeyID]Base64String
	if err := json.Unmarshal(message, &object); err != nil {
		return nil, err
	}

	// We don't sign the contents of the unsigned key so we remove it.
	rawUnsigned, hasUnsigned := object["unsigned"]
	delete(object, "unsigned")

	// Parse the existing signatures if they exist.
	// Signing a JSON object adds our signature to the existing
	// signature rather than replacing it.
	if rawSignatures := object["signatures"]; rawSignatures != nil {
		if err := json.Unmarshal(*rawSignatures, &signatures); err != nil {
			return nil, err
		}
		delete(object, "signatures")
	} else {
		signatures = map[string]map[KeyID]Base64String{}
	}

	// Encode the JSON object without the "signatures" key or
	// the "unsigned" key in the canonical format.
	unsorted, err := json.Marshal(object)
	if err != nil {
		return nil, err
	}
	canonical, err := CanonicalJSON(unsorted)
	if err != nil {
		return nil, err
	}

	// Sign the canonical JSON with the ed25519 key.
	signature := Base64String(ed25519.Sign(privateKey, canonical))

	// Add the signature to the "signature" key.
	signaturesForEntity := signatures[signingName]
	if signaturesForEntity != nil {
		signaturesForEntity[keyID] = signature
	} else {
		signatures[signingName] = map[KeyID]Base64String{keyID: signature}
	}
	var rawSignatures json.RawMessage
	rawSignatures, err = json.Marshal(signatures)
	if err != nil {
		return nil, err
	}
	object["signatures"] = &rawSignatures

	// Add the unsigned key back if it was present.
	if hasUnsigned {
		object["unsigned"] = rawUnsigned
	}

	return json.Marshal(object)
}

// ListKeyIDs lists the key IDs a given entity has signed a message with.
func ListKeyIDs(signingName string, message []byte) ([]KeyID, error) {
	var object struct {
		Signatures map[string]map[KeyID]json.RawMessage `json:"signatures"`
	}
	if err := json.Unmarshal(message, &object); err != nil {
		return nil, err
	}
	var result []KeyID
	for keyID := range object.Signatures[signingName] {
		result = append(result, keyID)
	}
	return result, nil
}

// VerifyJSON checks that the entity has signed the message using a particular key.
func VerifyJSON(signingName string, keyID KeyID, publicKey ed25519.PublicKey, message []byte) error {
	// Unpack the top-level key of the JSON object without unpacking the contents of the keys.
	// This allows us to add and remove the top-level keys from the JSON object.
	// It also ensures that the JSON is actually a valid JSON object.
	var object map[string]*json.RawMessage
	var signatures map[string]map[KeyID]Base64String
	if err := json.Unmarshal(message, &object); err != nil {
		return err
	}

	// Check that there is a signature from the entity that we are expecting a signature from.
	if object["signatures"] == nil {
		return fmt.Errorf("No signatures")
	}
	if err := json.Unmarshal(*object["signatures"], &signatures); err != nil {
		return err
	}
	signature, ok := signatures[signingName][keyID]
	if !ok {
		return fmt.Errorf("No signature from %q with ID %q", signingName, keyID)
	}
	if len(signature) != ed25519.SignatureSize {
		return fmt.Errorf("Bad signature length from %q with ID %q", signingName, keyID)
	}

	// The "unsigned" key and "signatures" keys aren't covered by the signature so remove them.
	delete(object, "unsigned")
	delete(object, "signatures")

	// Encode the JSON without the "unsigned" and "signatures" keys in the canonical format.
	unsorted, err := json.Marshal(object)
	if err != nil {
		return err
	}
	canonical, err := CanonicalJSON(unsorted)
	if err != nil {
		return err
	}

	// Verify the ed25519 signature.
	if !ed25519.Verify(publicKey, canonical, signature) {
		return fmt.Errorf("Bad signature from %q with ID %q", signingName, keyID)
	}

	return nil
}
