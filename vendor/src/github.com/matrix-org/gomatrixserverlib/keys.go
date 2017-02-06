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
	"io/ioutil"
	"net"
	"net/http"
	"strings"
	"time"
)

// ServerKeys are the ed25519 signing keys published by a matrix server.
// Contains SHA256 fingerprints of the TLS X509 certificates used by the server.
type ServerKeys struct {
	Raw             []byte     `json:"-"`           // Copy of the raw JSON for signature checking.
	ServerName      string     `json:"server_name"` // The name of the server.
	TLSFingerprints []struct { // List of SHA256 fingerprints of X509 certificates.
		SHA256 Base64String `json:"sha256"`
	} `json:"tls_fingerprints"`
	VerifyKeys map[string]struct { // The current signing keys in use on this server.
		Key Base64String `json:"key"` // The public key.
	} `json:"verify_keys"`
	ValidUntilTS  int64               `json:"valid_until_ts"` // When this result is valid until in milliseconds.
	OldVerifyKeys map[string]struct { // Old keys that are now only valid for checking historic events.
		Key       Base64String `json:"key"`        // The public key.
		ExpiredTS uint64       `json:"expired_ts"` // When this key stopped being valid for event signing.
	} `json:"old_verify_keys"`
}

// FetchKeysDirect fetches the matrix keys directly from the given address.
// Optionally sets a SNI header if ``sni`` is not empty.
// Returns the server keys and the state of the TLS connection used to retrieve them.
func FetchKeysDirect(serverName, addr, sni string) (*ServerKeys, *tls.ConnectionState, error) {
	// Create a TLS connection.
	tcpconn, err := net.Dial("tcp", addr)
	if err != nil {
		return nil, nil, err
	}
	defer tcpconn.Close()
	tlsconn := tls.Client(tcpconn, &tls.Config{
		ServerName:         sni,
		InsecureSkipVerify: true, // This must be specified even though the TLS library will ignore it.
	})
	if err = tlsconn.Handshake(); err != nil {
		return nil, nil, err
	}
	connectionState := tlsconn.ConnectionState()

	// Write a GET /_matrix/key/v2/server down the connection.
	requestURL := "matrix://" + serverName + "/_matrix/key/v2/server"
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
		defer response.Body.Close()
	}
	if err != nil {
		return nil, nil, err
	}
	var keys ServerKeys
	if keys.Raw, err = ioutil.ReadAll(response.Body); err != nil {
		return nil, nil, err
	}
	if err = json.Unmarshal(keys.Raw, &keys); err != nil {
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
	AllChecksOK               bool                     // Did all the checks pass?
	MatchingServerName        bool                     // Does the server name match what was requested.
	FutureValidUntilTS        bool                     // The valid until TS is in the future.
	HasEd25519Key             bool                     // The server has at least one ed25519 key.
	AllEd25519ChecksOK        *bool                    // All the Ed25519 checks are ok. or null if there weren't any to check.
	Ed25519Checks             map[string]Ed25519Checks // Checks for Ed25519 keys.
	HasTLSFingerprint         bool                     // The server has at least one fingerprint.
	AllTLSFingerprintChecksOK *bool                    // All the fingerpint checks are ok.
	TLSFingerprintChecks      []TLSFingerprintChecks   // Checks for TLS fingerprints.
	MatchingTLSFingerprint    *bool                    // The TLS fingerprint for the connection matches one of the listed fingerprints.
}

// CheckKeys checks the keys returned from a server to make sure they are valid.
// If the checks pass then also return a map of key_id to Ed25519 public key and a list of SHA256 TLS fingerprints.
func CheckKeys(serverName string, now time.Time, keys ServerKeys, connState *tls.ConnectionState) (
	checks KeyChecks, ed25519Keys map[string]Base64String, sha256Fingerprints []Base64String,
) {
	checks.MatchingServerName = serverName == keys.ServerName
	checks.FutureValidUntilTS = now.UnixNano() < keys.ValidUntilTS*1000000
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
		if bytes.Compare(digest[:], fingerprint) == 0 {
			return true
		}
	}
	return false
}

func checkVerifyKeys(keys ServerKeys, checks *KeyChecks) map[string]Base64String {
	allEd25519ChecksOK := true
	checks.Ed25519Checks = map[string]Ed25519Checks{}
	verifyKeys := map[string]Base64String{}
	for keyID, keyData := range keys.VerifyKeys {
		algorithm := strings.SplitN(keyID, ":", 2)[0]
		publicKey := keyData.Key
		if algorithm == "ed25519" {
			checks.HasEd25519Key = true
			checks.AllEd25519ChecksOK = &allEd25519ChecksOK
			entry := Ed25519Checks{
				ValidEd25519: len(publicKey) == 32,
			}
			if entry.ValidEd25519 {
				err := VerifyJSON(keys.ServerName, keyID, []byte(publicKey), keys.Raw)
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
