package gomatrixserverlib

import (
	"fmt"
	"golang.org/x/crypto/ed25519"
	"strings"
	"time"
)

// A PublicKeyRequest is a request for a public key with a particular key ID.
type PublicKeyRequest struct {
	// The server to fetch a key for.
	ServerName ServerName
	// The ID of the key to fetch.
	KeyID KeyID
}

// A KeyFetcher is a way of fetching public keys in bulk.
type KeyFetcher interface {
	// Lookup a batch of public keys.
	// Takes a map from (server name, key ID) pairs to timestamp.
	// The timestamp is when the keys need to be vaild up to.
	// Returns a map from (server name, key ID) pairs to server key objects for
	// that server name containing that key ID
	// The result may have fewer (server name, key ID) pairs than were in the request.
	// The result may have more (server name, key ID) pairs than were in the request.
	// Returns an error if there was a problem fetching the keys.
	FetchKeys(requests map[PublicKeyRequest]Timestamp) (map[PublicKeyRequest]ServerKeys, error)
}

// A KeyDatabase is a store for caching public keys.
type KeyDatabase interface {
	KeyFetcher
	// Add a block of public keys to the database.
	StoreKeys(map[PublicKeyRequest]ServerKeys) error
}

// A KeyRing stores keys for matrix servers and provides methods for verifying JSON messages.
type KeyRing struct {
	KeyFetchers []KeyFetcher
	KeyDatabase KeyDatabase
}

// A VerifyJSONRequest is a request to check for a signature on a JSON message.
// A JSON message is valid for a server if the message has at least one valid
// signature from that server.
type VerifyJSONRequest struct {
	// The name of the matrix server to check for a signature for.
	ServerName ServerName
	// The millisecond posix timestamp the message needs to be valid at.
	AtTS Timestamp
	// The JSON bytes.
	Message []byte
}

// A VerifyJSONResult is the result of checking the signature of a JSON message.
type VerifyJSONResult struct {
	// Whether the message passed the signature checks.
	// This will be nil if the message passed the checks.
	// This will have an error if the message did not pass the checks.
	Error error
}

// VerifyJSONs performs bulk JSON signature verification for a list of VerifyJSONRequests.
// Returns a list of VerifyJSONResults with the same length and order as the request list.
// The caller should check the Result field for each entry to see if it was valid.
// Returns an error if there was a problem talking to the database or one of the other methods
// of fetching the public keys.
func (k *KeyRing) VerifyJSONs(requests []VerifyJSONRequest) ([]VerifyJSONResult, error) {
	results := make([]VerifyJSONResult, len(requests))
	keyIDs := make([][]KeyID, len(requests))

	for i := range requests {
		ids, err := ListKeyIDs(string(requests[i].ServerName), requests[i].Message)
		if err != nil {
			results[i].Error = fmt.Errorf("gomatrixserverlib: error extracting key IDs")
			continue
		}
		for _, keyID := range ids {
			if k.isAlgorithmSupported(keyID) {
				keyIDs[i] = append(keyIDs[i], keyID)
			}
		}
		if len(keyIDs[i]) == 0 {
			results[i].Error = fmt.Errorf(
				"gomatrixserverlib: not signed by %q with a supported algorithm", requests[i].ServerName,
			)
			continue
		}
		// Set a place holder error in the result field.
		// This will be unset if one of the signature checks passes.
		// This will be overwritten if one of the signature checks fails.
		// Therefore this will only remain in place if the keys couldn't be downloaded.
		results[i].Error = fmt.Errorf(
			"gomatrixserverlib: could not download key for %q", requests[i].ServerName,
		)
	}

	keyRequests := k.publicKeyRequests(requests, results, keyIDs)
	if len(keyRequests) == 0 {
		// There aren't any keys to fetch so we can stop here.
		// This will happen if all the objects are missing supported signatures.
		return results, nil
	}
	keysFromDatabase, err := k.KeyDatabase.FetchKeys(keyRequests)
	if err != nil {
		return nil, err
	}
	k.checkUsingKeys(requests, results, keyIDs, keysFromDatabase)

	for i := range k.KeyFetchers {
		keyRequests := k.publicKeyRequests(requests, results, keyIDs)
		if len(keyRequests) == 0 {
			// There aren't any keys to fetch so we can stop here.
			// This means that we've checked every JSON object we can check.
			return results, nil
		}
		// TODO: Coalesce in-flight requests for the same keys.
		// Otherwise we risk spamming the servers we query the keys from.
		keysFetched, err := k.KeyFetchers[i].FetchKeys(keyRequests)
		if err != nil {
			return nil, err
		}
		k.checkUsingKeys(requests, results, keyIDs, keysFetched)

		// Add the keys to the database so that we won't need to fetch them again.
		if err := k.KeyDatabase.StoreKeys(keysFetched); err != nil {
			return nil, err
		}
	}

	return results, nil
}

func (k *KeyRing) isAlgorithmSupported(keyID KeyID) bool {
	return strings.HasPrefix(string(keyID), "ed25519:")
}

func (k *KeyRing) publicKeyRequests(requests []VerifyJSONRequest, results []VerifyJSONResult, keyIDs [][]KeyID) map[PublicKeyRequest]Timestamp {
	keyRequests := map[PublicKeyRequest]Timestamp{}
	for i := range requests {
		if results[i].Error == nil {
			// We've already verified this message, we don't need to refetch the keys for it.
			continue
		}
		for _, keyID := range keyIDs[i] {
			k := PublicKeyRequest{requests[i].ServerName, keyID}
			// Grab the maximum neeeded TS for this server and key ID.
			// This will default to 0 if the server and keyID weren't in the map.
			maxTS := keyRequests[k]
			if maxTS <= requests[i].AtTS {
				// We clobber on equality since that means that if the server and keyID
				// weren't already in the map and since AtTS is unsigned and since the
				// default value for maxTS is 0 we will always insert an entry for the
				// server and keyID.
				keyRequests[k] = requests[i].AtTS
			}
		}
	}
	return keyRequests
}

func (k *KeyRing) checkUsingKeys(
	requests []VerifyJSONRequest, results []VerifyJSONResult, keyIDs [][]KeyID,
	keys map[PublicKeyRequest]ServerKeys,
) {
	for i := range requests {
		if results[i].Error == nil {
			// We've already checked this message and it passed the signature checks.
			// So we can skip to the next message.
			continue
		}
		for _, keyID := range keyIDs[i] {
			serverKeys, ok := keys[PublicKeyRequest{requests[i].ServerName, keyID}]
			if !ok {
				// No key for this key ID so we continue onto the next key ID.
				continue
			}
			publicKey := serverKeys.PublicKey(keyID, requests[i].AtTS)
			if publicKey == nil {
				// The key wasn't valid at the timestamp we needed it to be valid at.
				// So skip onto the next key.
				results[i].Error = fmt.Errorf(
					"gomatrixserverlib: key with ID %q for %q not valid at %d",
					keyID, requests[i].ServerName, requests[i].AtTS,
				)
				continue
			}
			if err := VerifyJSON(
				string(requests[i].ServerName), keyID, ed25519.PublicKey(publicKey), requests[i].Message,
			); err != nil {
				// The signature wasn't valid, record the error and try the next key ID.
				results[i].Error = err
				continue
			}
			// The signature is valid, set the result to nil.
			results[i].Error = nil
			break
		}
	}
}

// A PerspectiveKeyFetcher fetches server keys from a single perspective server.
type PerspectiveKeyFetcher struct {
	// The name of the perspective server to fetch keys from.
	PerspectiveServerName ServerName
	// The ed25519 public keys the perspective server must sign responses with.
	PerspectiveServerKeys map[KeyID]ed25519.PublicKey
	// The federation client to use to fetch keys with.
	Client Client
}

// FetchKeys implements KeyFetcher
func (p *PerspectiveKeyFetcher) FetchKeys(requests map[PublicKeyRequest]Timestamp) (map[PublicKeyRequest]ServerKeys, error) {
	results, err := p.Client.LookupServerKeys(p.PerspectiveServerName, requests)
	if err != nil {
		return nil, err
	}

	for req, keys := range results {
		var valid bool
		keyIDs, err := ListKeyIDs(string(p.PerspectiveServerName), keys.Raw)
		if err != nil {
			// The response from the perspective server was corrupted.
			return nil, err
		}
		for _, keyID := range keyIDs {
			perspectiveKey, ok := p.PerspectiveServerKeys[keyID]
			if !ok {
				// We don't have a key for that keyID, skip to the next keyID.
				continue
			}
			if err := VerifyJSON(string(p.PerspectiveServerName), keyID, perspectiveKey, keys.Raw); err != nil {
				// An invalid signature is very bad since it means we have a
				// problem talking to the perspective server.
				return nil, err
			}
			valid = true
			break
		}
		if !valid {
			// This means we don't have a known signature from the perspective server.
			return nil, fmt.Errorf("gomatrixserverlib: not signed with a known key for the perspective server")
		}

		// Check that the keys are valid for the server.
		checks, _, _ := CheckKeys(req.ServerName, time.Unix(0, 0), keys, nil)
		if !checks.AllChecksOK {
			// This is bad because it means that the perspective server was trying to feed us an invalid response.
			return nil, fmt.Errorf("gomatrixserverlib: key response from perspective server failed checks")
		}
	}

	return results, nil
}

// A DirectKeyFetcher fetches keys directly from a server.
// This may be suitable for local deployments that are firewalled from the public internet where DNS can be trusted.
type DirectKeyFetcher struct {
	// The federation client to use to fetch keys with.
	Client Client
}

// FetchKeys implements KeyFetcher
func (d *DirectKeyFetcher) FetchKeys(requests map[PublicKeyRequest]Timestamp) (map[PublicKeyRequest]ServerKeys, error) {
	byServer := map[ServerName]map[PublicKeyRequest]Timestamp{}
	for req, ts := range requests {
		server := byServer[req.ServerName]
		if server == nil {
			server = map[PublicKeyRequest]Timestamp{}
			byServer[req.ServerName] = server
		}
		server[req] = ts
	}

	results := map[PublicKeyRequest]ServerKeys{}
	for server, reqs := range byServer {
		// TODO: make these requests in parallel
		serverResults, err := d.fetchKeysForServer(server, reqs)
		if err != nil {
			// TODO: Should we actually be erroring here? or should we just drop those keys from the result map?
			return nil, err
		}
		for req, keys := range serverResults {
			results[req] = keys
		}
	}
	return results, nil
}

func (d *DirectKeyFetcher) fetchKeysForServer(
	serverName ServerName, requests map[PublicKeyRequest]Timestamp,
) (map[PublicKeyRequest]ServerKeys, error) {
	results, err := d.Client.LookupServerKeys(serverName, requests)
	if err != nil {
		return nil, err
	}

	for req, keys := range results {
		// Check that the keys are valid for the server.
		checks, _, _ := CheckKeys(req.ServerName, time.Unix(0, 0), keys, nil)
		if !checks.AllChecksOK {
			return nil, fmt.Errorf("gomatrixserverlib: key response direct from %q failed checks", serverName)
		}
	}

	return results, nil
}
