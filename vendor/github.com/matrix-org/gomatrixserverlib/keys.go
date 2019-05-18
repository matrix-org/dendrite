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
	"bufio"
	"bytes"
	"crypto/sha256"
	"crypto/tls"
	"encoding/json"
	"net"
	"net/http"
	"strings"
	"time"
)

// A ServerName is the name a matrix homeserver is identified by.
// It is a DNS name without a trailing dot optionally followed by a port.
// So it has the format: "[0-9A-Za-z\-]+(\.[0-9A-Za-z\-]+)*(:[0-9]+)?"
type ServerName string

// ServerKeys are the ed25519 signing keys published by a matrix server.
// Contains SHA256 fingerprints of the TLS X509 certificates used by the server.
type ServerKeys struct {
	// Copy of the raw JSON for signature checking.
	Raw []byte
	// The decoded JSON fields.
	ServerKeyFields
}

// A TLSFingerprint is a SHA256 hash of an X509 certificate.
type TLSFingerprint struct {
	SHA256 Base64String `json:"sha256"`
}

// A VerifyKey is a ed25519 public key for a server.
type VerifyKey struct {
	// The public key.
	Key Base64String `json:"key"`
}

// An OldVerifyKey is an old ed25519 public key that is no longer valid.
type OldVerifyKey struct {
	VerifyKey
	// When this key stopped being valid for event signing in milliseconds.
	ExpiredTS Timestamp `json:"expired_ts"`
}

// ServerKeyFields are the parsed JSON contents of the ed25519 signing keys published by a matrix server.
type ServerKeyFields struct {
	// The name of the server
	ServerName ServerName `json:"server_name"`
	// List of SHA256 fingerprints of X509 certificates used by this server.
	TLSFingerprints []TLSFingerprint `json:"tls_fingerprints"`
	// The current signing keys in use on this server.
	// The keys of the map are the IDs of the keys.
	// These are valid while this response is valid.
	VerifyKeys map[KeyID]VerifyKey `json:"verify_keys"`
	// When this result is valid until in milliseconds.
	ValidUntilTS Timestamp `json:"valid_until_ts"`
	// Old keys that are now only valid for checking historic events.
	// The keys of the map are the IDs of the keys.
	OldVerifyKeys map[KeyID]OldVerifyKey `json:"old_verify_keys"`
}

// UnmarshalJSON implements json.Unmarshaler
func (keys *ServerKeys) UnmarshalJSON(data []byte) error {
	keys.Raw = data
	return json.Unmarshal(data, &keys.ServerKeyFields)
}

// MarshalJSON implements json.Marshaler
func (keys ServerKeys) MarshalJSON() ([]byte, error) {
	// We already have a copy of the serialised JSON for the keys so we can return that directly.
	return keys.Raw, nil
}

// PublicKey returns a public key with the given ID valid at the given TS or nil if no such key exists.
func (keys ServerKeys) PublicKey(keyID KeyID, atTS Timestamp) []byte {
	if currentKey, ok := keys.VerifyKeys[keyID]; ok && (atTS <= keys.ValidUntilTS) {
		return currentKey.Key
	}
	if oldKey, ok := keys.OldVerifyKeys[keyID]; ok && (atTS <= oldKey.ExpiredTS) {
		return oldKey.Key
	}
	return nil
}

// FetchKeysDirect fetches the matrix keys directly from the given address.
// Optionally sets a SNI header if ``sni`` is not empty.
// Returns the server keys and the state of the TLS connection used to retrieve them.
func FetchKeysDirect(serverName ServerName, addr, sni string) (*ServerKeys, *tls.ConnectionState, error) {
	// Create a TLS connection.
	tcpconn, err := net.Dial("tcp", addr)
	if err != nil {
		return nil, nil, err
	}
	defer tcpconn.Close() // nolint: errcheck
	tlsconn := tls.Client(tcpconn, &tls.Config{
		ServerName: sni,

		// This must be specified even though the TLS library will ignore it.
		InsecureSkipVerify: true, // nolint: gas
	})
	if err = tlsconn.Handshake(); err != nil {
		return nil, nil, err
	}
	connectionState := tlsconn.ConnectionState()

	// Write a GET /_matrix/key/v2/server down the connection.
	requestURL := "matrix://" + string(serverName) + "/_matrix/key/v2/server"
	request, err := http.NewRequest("GET", requestURL, nil)
	if err != nil {
		return nil, nil, err
	}
	request.Header.Set("Connection", "close")
	if err = request.Write(tlsconn); err != nil {
		return nil, nil, err
	}

	// Read the 200 OK from the server.
	response, err := http.ReadResponse(bufio.NewReader(tlsconn), request)
	if response != nil {
		defer response.Body.Close() // nolint: errcheck
	}
	if err != nil {
		return nil, nil, err
	}
	var keys ServerKeys
	if err = json.NewDecoder(response.Body).Decode(&keys); err != nil {
		return nil, nil, err
	}
	return &keys, &connectionState, nil
}

// Ed25519Checks are the checks that are applied to Ed25519 keys in ServerKey responses.
type Ed25519Checks struct {
	ValidEd25519      bool // The verify key is valid Ed25519 keys.
	MatchingSignature bool // The verify key has a valid signature.
}

// TLSFingerprintChecks are the checks that are applied to TLS fingerprints in ServerKey responses.
type TLSFingerprintChecks struct {
	ValidSHA256 bool // The TLS fingerprint includes a valid SHA-256 hash.
}

// KeyChecks are the checks that should be applied to ServerKey responses.
type KeyChecks struct {
	AllChecksOK               bool                    // Did all the checks pass?
	MatchingServerName        bool                    // Does the server name match what was requested.
	FutureValidUntilTS        bool                    // The valid until TS is in the future.
	HasEd25519Key             bool                    // The server has at least one ed25519 key.
	HasTLSFingerprint         bool                    // The server has at least one fingerprint.
	AllEd25519ChecksOK        *bool                   // All the Ed25519 checks are ok. or null if there weren't any to check.
	Ed25519Checks             map[KeyID]Ed25519Checks // Checks for Ed25519 keys.
	AllTLSFingerprintChecksOK *bool                   // All the fingerprint checks are ok.
	TLSFingerprintChecks      []TLSFingerprintChecks  // Checks for TLS fingerprints.
	MatchingTLSFingerprint    *bool                   // The TLS fingerprint for the connection matches one of the listed fingerprints.
}

// CheckKeys checks the keys returned from a server to make sure they are valid.
// If the checks pass then also return a map of key_id to Ed25519 public key and a list of SHA256 TLS fingerprints.
func CheckKeys(serverName ServerName, now time.Time, keys ServerKeys, connState *tls.ConnectionState) (
	checks KeyChecks, ed25519Keys map[KeyID]Base64String, sha256Fingerprints []Base64String,
) {
	checks.MatchingServerName = serverName == keys.ServerName
	checks.FutureValidUntilTS = keys.ValidUntilTS.Time().After(now)
	checks.AllChecksOK = checks.MatchingServerName && checks.FutureValidUntilTS

	ed25519Keys = checkVerifyKeys(keys, &checks)
	sha256Fingerprints = checkTLSFingerprints(keys, &checks)

	// Only check the fingerprint if we have the TLS connection state.
	if connState != nil {
		// Check the peer certificates.
		matches := checkFingerprint(connState, sha256Fingerprints)
		checks.MatchingTLSFingerprint = &matches
		checks.AllChecksOK = checks.AllChecksOK && matches
	}

	if !checks.AllChecksOK {
		sha256Fingerprints = nil
		ed25519Keys = nil
	}
	return
}

func checkFingerprint(connState *tls.ConnectionState, sha256Fingerprints []Base64String) bool {
	if len(connState.PeerCertificates) == 0 {
		return false
	}
	cert := connState.PeerCertificates[0]
	digest := sha256.Sum256(cert.Raw)
	for _, fingerprint := range sha256Fingerprints {
		if bytes.Equal(digest[:], fingerprint) {
			return true
		}
	}
	return false
}

func checkVerifyKeys(keys ServerKeys, checks *KeyChecks) map[KeyID]Base64String {
	allEd25519ChecksOK := true
	checks.Ed25519Checks = map[KeyID]Ed25519Checks{}
	verifyKeys := map[KeyID]Base64String{}
	for keyID, keyData := range keys.VerifyKeys {
		algorithm := strings.SplitN(string(keyID), ":", 2)[0]
		publicKey := keyData.Key
		if algorithm == "ed25519" {
			checks.HasEd25519Key = true
			checks.AllEd25519ChecksOK = &allEd25519ChecksOK
			entry := Ed25519Checks{
				ValidEd25519: len(publicKey) == 32,
			}
			if entry.ValidEd25519 {
				err := VerifyJSON(string(keys.ServerName), keyID, []byte(publicKey), keys.Raw)
				entry.MatchingSignature = err == nil
			}
			checks.Ed25519Checks[keyID] = entry
			if entry.MatchingSignature {
				verifyKeys[keyID] = publicKey
			} else {
				allEd25519ChecksOK = false
			}
		}
	}
	if checks.AllChecksOK {
		checks.AllChecksOK = checks.HasEd25519Key && allEd25519ChecksOK
	}
	return verifyKeys
}

func checkTLSFingerprints(keys ServerKeys, checks *KeyChecks) []Base64String {
	var fingerprints []Base64String
	allTLSFingerprintChecksOK := true
	for _, fingerprint := range keys.TLSFingerprints {
		checks.HasTLSFingerprint = true
		checks.AllTLSFingerprintChecksOK = &allTLSFingerprintChecksOK
		entry := TLSFingerprintChecks{
			ValidSHA256: len(fingerprint.SHA256) == sha256.Size,
		}
		checks.TLSFingerprintChecks = append(checks.TLSFingerprintChecks, entry)
		if entry.ValidSHA256 {
			fingerprints = append(fingerprints, fingerprint.SHA256)
		} else {
			allTLSFingerprintChecksOK = false
		}
	}
	if checks.AllChecksOK {
		checks.AllChecksOK = checks.HasTLSFingerprint && allTLSFingerprintChecksOK
	}
	return fingerprints
}
