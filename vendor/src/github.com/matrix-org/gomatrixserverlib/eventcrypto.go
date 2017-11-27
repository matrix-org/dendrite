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
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"

	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
	"golang.org/x/crypto/ed25519"
)

// addContentHashesToEvent sets the "hashes" key of the event with a SHA-256 hash of the unredacted event content.
// This hash is used to detect whether the unredacted content of the event is valid.
// Returns the event JSON with a "hashes" key added to it.
func addContentHashesToEvent(eventJSON []byte) ([]byte, error) {
	var event map[string]rawJSON

	if err := json.Unmarshal(eventJSON, &event); err != nil {
		return nil, err
	}

	unsignedJSON := event["unsigned"]

	delete(event, "unsigned")
	delete(event, "hashes")

	hashableEventJSON, err := json.Marshal(event)
	if err != nil {
		return nil, err
	}

	hashableEventJSON, err = CanonicalJSON(hashableEventJSON)
	if err != nil {
		return nil, err
	}

	sha256Hash := sha256.Sum256(hashableEventJSON)
	hashes := struct {
		Sha256 Base64String `json:"sha256"`
	}{Base64String(sha256Hash[:])}
	hashesJSON, err := json.Marshal(&hashes)
	if err != nil {
		return nil, err
	}

	if len(unsignedJSON) > 0 {
		event["unsigned"] = unsignedJSON
	}
	event["hashes"] = rawJSON(hashesJSON)

	return json.Marshal(event)
}

// checkEventContentHash checks if the unredacted content of the event matches the SHA-256 hash under the "hashes" key.
// Assumes that eventJSON has been canonicalised already.
func checkEventContentHash(eventJSON []byte) error {
	var err error

	result := gjson.GetBytes(eventJSON, "hashes.sha256")
	var hash Base64String
	if err = hash.Decode(result.Str); err != nil {
		return err
	}

	hashableEventJSON := eventJSON

	for _, key := range []string{"signatures", "unsigned", "hashes"} {
		if hashableEventJSON, err = sjson.DeleteBytes(hashableEventJSON, key); err != nil {
			return err
		}
	}

	sha256Hash := sha256.Sum256(hashableEventJSON)

	if !bytes.Equal(sha256Hash[:], []byte(hash)) {
		return fmt.Errorf("Invalid Sha256 content hash: %v != %v", sha256Hash[:], []byte(hash))
	}

	return nil
}

// ReferenceSha256HashOfEvent returns the SHA-256 hash of the redacted event content.
// This is used when referring to this event from other events.
func referenceOfEvent(eventJSON []byte) (EventReference, error) {
	redactedJSON, err := redactEvent(eventJSON)
	if err != nil {
		return EventReference{}, err
	}

	var event map[string]rawJSON
	if err = json.Unmarshal(redactedJSON, &event); err != nil {
		return EventReference{}, err
	}

	delete(event, "signatures")
	delete(event, "unsigned")

	hashableEventJSON, err := json.Marshal(event)
	if err != nil {
		return EventReference{}, err
	}

	hashableEventJSON, err = CanonicalJSON(hashableEventJSON)
	if err != nil {
		return EventReference{}, err
	}

	sha256Hash := sha256.Sum256(hashableEventJSON)

	var eventID string
	if err = json.Unmarshal(event["event_id"], &eventID); err != nil {
		return EventReference{}, err
	}

	return EventReference{eventID, sha256Hash[:]}, nil
}

// SignEvent adds a ED25519 signature to the event for the given key.
func signEvent(signingName string, keyID KeyID, privateKey ed25519.PrivateKey, eventJSON []byte) ([]byte, error) {

	// Redact the event before signing so signature that will remain valid even if the event is redacted.
	redactedJSON, err := redactEvent(eventJSON)
	if err != nil {
		return nil, err
	}

	// Sign the JSON, this adds a "signatures" key to the redacted event.
	// TODO: Make an internal version of SignJSON that returns just the signatures so that we don't have to parse it out of the JSON.
	signedJSON, err := SignJSON(signingName, keyID, privateKey, redactedJSON)
	if err != nil {
		return nil, err
	}

	var signedEvent struct {
		Signatures rawJSON `json:"signatures"`
	}
	if err := json.Unmarshal(signedJSON, &signedEvent); err != nil {
		return nil, err
	}

	// Unmarshal the event JSON so that we can replace the signatures key.
	var event map[string]rawJSON
	if err := json.Unmarshal(eventJSON, &event); err != nil {
		return nil, err
	}

	event["signatures"] = signedEvent.Signatures

	return json.Marshal(event)
}

// VerifyEventSignature checks if the event has been signed by the given ED25519 key.
func verifyEventSignature(signingName string, keyID KeyID, publicKey ed25519.PublicKey, eventJSON []byte) error {
	redactedJSON, err := redactEvent(eventJSON)
	if err != nil {
		return err
	}

	return VerifyJSON(signingName, keyID, publicKey, redactedJSON)
}

// VerifyEventSignatures checks that each event in a list of events has valid
// signatures from the server that sent it.
func VerifyEventSignatures(ctx context.Context, events []Event, keyRing JSONVerifier) error { // nolint: gocyclo
	var toVerify []VerifyJSONRequest
	for _, event := range events {
		redactedJSON, err := redactEvent(event.eventJSON)
		if err != nil {
			return err
		}

		domains := make(map[ServerName]bool)
		domains[event.Origin()] = true

		// in general, we expect the domain of the sender id to be the
		// same as the origin; however there was a bug in an old version
		// of synapse which meant that some joins/leaves used the origin
		// and event id supplied by the helping server instead of the
		// joining/leaving server.
		//
		// That's ok, provided it's signed by the sender's server too.
		//
		// XXX we may have to exclude 3pid invites here, as per
		// https://github.com/matrix-org/synapse/blob/v0.21.0/synapse/event_auth.py#L58-L64.
		//
		senderDomain, err := domainFromID(event.Sender())
		if err != nil {
			return err
		}
		domains[ServerName(senderDomain)] = true

		// MRoomMember invite events are signed by both the server sending
		// the invite and the server the invite is for.
		if event.Type() == MRoomMember && event.StateKey() != nil {
			targetDomain, err := domainFromID(*event.StateKey())
			if err != nil {
				return err
			}
			if ServerName(targetDomain) != event.Origin() {
				c, err := newMemberContentFromEvent(event)
				if err != nil {
					return err
				}
				if c.Membership == invite {
					domains[ServerName(targetDomain)] = true
				}
			}
		}

		for domain := range domains {
			v := VerifyJSONRequest{
				Message:    redactedJSON,
				AtTS:       event.OriginServerTS(),
				ServerName: domain,
			}
			toVerify = append(toVerify, v)
		}
	}

	results, err := keyRing.VerifyJSONs(ctx, toVerify)
	if err != nil {
		return err
	}

	// Check that all the event JSON was correctly signed.
	for _, result := range results {
		if result.Error != nil {
			return result.Error
		}
	}

	// Everything was okay.
	return nil
}
