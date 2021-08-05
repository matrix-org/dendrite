// Copyright 2021 The Matrix.org Foundation C.I.C.
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

package internal

import (
	"context"
	"crypto/ed25519"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/matrix-org/dendrite/keyserver/api"
	"github.com/matrix-org/dendrite/keyserver/types"
	"github.com/matrix-org/gomatrixserverlib"
	"github.com/sirupsen/logrus"
)

func sanityCheckKey(key gomatrixserverlib.CrossSigningKey, userID string, purpose gomatrixserverlib.CrossSigningKeyPurpose) error {
	// Is there exactly one key?
	if len(key.Keys) != 1 {
		return fmt.Errorf("should contain exactly one key")
	}

	// Does the key ID match the key value? Iterates exactly once
	for keyID, keyData := range key.Keys {
		b64 := keyData.Encode()
		tokens := strings.Split(string(keyID), ":")
		if len(tokens) != 2 {
			return fmt.Errorf("key ID is incorrectly formatted")
		}
		if tokens[1] != b64 {
			return fmt.Errorf("key ID isn't correct")
		}
	}

	// Does the key claim to be from the right user?
	if userID != key.UserID {
		return fmt.Errorf("key has a user ID mismatch")
	}

	// Does the key contain the correct purpose?
	useful := false
	for _, usage := range key.Usage {
		if usage == purpose {
			useful = true
			break
		}
	}
	if !useful {
		return fmt.Errorf("key does not contain correct usage purpose")
	}

	return nil
}

// nolint:gocyclo
func (a *KeyInternalAPI) PerformUploadDeviceKeys(ctx context.Context, req *api.PerformUploadDeviceKeysRequest, res *api.PerformUploadDeviceKeysResponse) {
	var masterKey gomatrixserverlib.Base64Bytes
	hasMasterKey := false

	if len(req.MasterKey.Keys) > 0 {
		if err := sanityCheckKey(req.MasterKey, req.UserID, gomatrixserverlib.CrossSigningKeyPurposeMaster); err != nil {
			res.Error = &api.KeyError{
				Err: "Master key sanity check failed: " + err.Error(),
			}
			return
		}
		hasMasterKey = true
		for _, keyData := range req.MasterKey.Keys { // iterates once, because sanityCheckKey requires one key
			masterKey = keyData
		}
	}

	if len(req.SelfSigningKey.Keys) > 0 {
		if err := sanityCheckKey(req.SelfSigningKey, req.UserID, gomatrixserverlib.CrossSigningKeyPurposeSelfSigning); err != nil {
			res.Error = &api.KeyError{
				Err: "Self-signing key sanity check failed: " + err.Error(),
			}
			return
		}
	}

	if len(req.UserSigningKey.Keys) > 0 {
		if err := sanityCheckKey(req.UserSigningKey, req.UserID, gomatrixserverlib.CrossSigningKeyPurposeUserSigning); err != nil {
			res.Error = &api.KeyError{
				Err: "User-signing key sanity check failed: " + err.Error(),
			}
			return
		}
	}

	// If the user hasn't given a new master key, then let's go and get their
	// existing keys from the database.
	if !hasMasterKey {
		existingKeys, err := a.DB.CrossSigningKeysForUser(ctx, req.UserID)
		if err != nil {
			res.Error = &api.KeyError{
				Err: "Retrieving cross-signing keys from database failed: " + err.Error(),
			}
			return
		}

		masterKey, hasMasterKey = existingKeys[gomatrixserverlib.CrossSigningKeyPurposeMaster]
	}

	// If the user isn't a local user and we haven't successfully found a key
	// through any local means then ask over federation.
	if !hasMasterKey {
		_, host, err := gomatrixserverlib.SplitID('@', req.UserID)
		if err != nil {
			res.Error = &api.KeyError{
				Err: "Retrieving cross-signing keys from federation failed: " + err.Error(),
			}
			return
		}
		keys, err := a.FedClient.QueryKeys(ctx, host, map[string][]string{
			req.UserID: {},
		})
		if err != nil {
			res.Error = &api.KeyError{
				Err: "Retrieving cross-signing keys from federation failed: " + err.Error(),
			}
			return
		}
		switch k := keys.MasterKeys[req.UserID].CrossSigningBody.(type) {
		case *gomatrixserverlib.CrossSigningKey:
			if err := sanityCheckKey(*k, req.UserID, gomatrixserverlib.CrossSigningKeyPurposeMaster); err != nil {
				res.Error = &api.KeyError{
					Err: "Master key sanity check failed: " + err.Error(),
				}
				return
			}
		default:
			res.Error = &api.KeyError{
				Err: "Unexpected type for master key retrieved from federation",
			}
			return
		}
	}

	// If we still don't have a master key at this point then there's nothing else
	// we can do - we've checked both the request and the database.
	if !hasMasterKey {
		res.Error = &api.KeyError{
			Err:            "No master key was found, either in the database or in the request!",
			IsMissingParam: true,
		}
		return
	}

	// The key ID is basically the key itself.
	masterKeyID := gomatrixserverlib.KeyID(fmt.Sprintf("ed25519:%s", masterKey.Encode()))

	// Work out which things we need to verify the signatures for.
	toVerify := make(map[gomatrixserverlib.CrossSigningKeyPurpose]gomatrixserverlib.CrossSigningKey, 3)
	toStore := types.CrossSigningKeyMap{}
	if len(req.MasterKey.Keys) > 0 {
		toVerify[gomatrixserverlib.CrossSigningKeyPurposeMaster] = req.MasterKey
	}
	if len(req.SelfSigningKey.Keys) > 0 {
		toVerify[gomatrixserverlib.CrossSigningKeyPurposeSelfSigning] = req.SelfSigningKey
	}
	if len(req.UserSigningKey.Keys) > 0 {
		toVerify[gomatrixserverlib.CrossSigningKeyPurposeUserSigning] = req.UserSigningKey
	}
	for purpose, key := range toVerify {
		// Collect together the key IDs we need to verify with. This will include
		// all of the key IDs specified in the signatures. We don't do this for
		// the master key because we have no means to verify the signatures - we
		// instead just need to store them.
		if purpose != gomatrixserverlib.CrossSigningKeyPurposeMaster {
			// Marshal the specific key back into JSON so that we can verify the
			// signature of it.
			keyJSON, err := json.Marshal(key)
			if err != nil {
				res.Error = &api.KeyError{
					Err: fmt.Sprintf("The JSON of the key section is invalid: %s", err.Error()),
				}
				return
			}

			// Now check if the subkey is signed by the master key.
			if err := gomatrixserverlib.VerifyJSON(req.UserID, masterKeyID, ed25519.PublicKey(masterKey), keyJSON); err != nil {
				res.Error = &api.KeyError{
					Err:                fmt.Sprintf("The %q sub-key failed master key signature verification: %s", purpose, err.Error()),
					IsInvalidSignature: true,
				}
				return
			}
		}

		// If we've reached this point then all the signatures are valid so
		// add the key to the list of keys to store.
		for _, keyData := range key.Keys { // iterates once, see sanityCheckKey
			toStore[purpose] = keyData
		}
	}

	if err := a.DB.StoreCrossSigningKeysForUser(ctx, req.UserID, toStore); err != nil {
		res.Error = &api.KeyError{
			Err: fmt.Sprintf("a.DB.StoreCrossSigningKeysForUser: %s", err),
		}
	}
}

func (a *KeyInternalAPI) PerformUploadDeviceSignatures(ctx context.Context, req *api.PerformUploadDeviceSignaturesRequest, res *api.PerformUploadDeviceSignaturesResponse) {
	selfSignatures := map[string]map[gomatrixserverlib.KeyID]gomatrixserverlib.CrossSigningForKeyOrDevice{}
	otherSignatures := map[string]map[gomatrixserverlib.KeyID]gomatrixserverlib.CrossSigningForKeyOrDevice{}

	for userID, forUserID := range req.Signatures {
		for keyID, keyOrDevice := range forUserID {
			switch key := keyOrDevice.CrossSigningBody.(type) {
			case *gomatrixserverlib.CrossSigningKey:
				if key.UserID == req.UserID {
					if _, ok := selfSignatures[userID]; !ok {
						selfSignatures[userID] = map[gomatrixserverlib.KeyID]gomatrixserverlib.CrossSigningForKeyOrDevice{}
					}
					selfSignatures[userID][keyID] = keyOrDevice
				} else {
					if _, ok := otherSignatures[userID]; !ok {
						otherSignatures[userID] = map[gomatrixserverlib.KeyID]gomatrixserverlib.CrossSigningForKeyOrDevice{}
					}
					otherSignatures[userID][keyID] = keyOrDevice
				}

			case *gomatrixserverlib.DeviceKeys:
				if key.UserID == req.UserID {
					if _, ok := selfSignatures[userID]; !ok {
						selfSignatures[userID] = map[gomatrixserverlib.KeyID]gomatrixserverlib.CrossSigningForKeyOrDevice{}
					}
					selfSignatures[userID][keyID] = keyOrDevice
				} else {
					if _, ok := otherSignatures[userID]; !ok {
						otherSignatures[userID] = map[gomatrixserverlib.KeyID]gomatrixserverlib.CrossSigningForKeyOrDevice{}
					}
					otherSignatures[userID][keyID] = keyOrDevice
				}

			default:
				continue
			}
		}
	}

	if err := a.processSelfSignatures(ctx, req.UserID, selfSignatures); err != nil {
		res.Error = &api.KeyError{
			Err: fmt.Sprintf("a.processSelfSignatures: %s", err),
		}
		return
	}

	if err := a.processOtherSignatures(ctx, req.UserID, otherSignatures); err != nil {
		res.Error = &api.KeyError{
			Err: fmt.Sprintf("a.processOtherSignatures: %s", err),
		}
		return
	}
}

func (a *KeyInternalAPI) processSelfSignatures(
	ctx context.Context, _ string,
	signatures map[string]map[gomatrixserverlib.KeyID]gomatrixserverlib.CrossSigningForKeyOrDevice,
) error {
	// Here we will process:
	// * The user signing their own devices using their self-signing key
	// * The user signing their master key using one of their devices

	for targetUserID, forTargetUserID := range signatures {
		for targetKeyID, signature := range forTargetUserID {
			switch sig := signature.CrossSigningBody.(type) {
			case *gomatrixserverlib.CrossSigningKey:
				for originUserID, forOriginUserID := range sig.Signatures {
					for originKeyID, originSig := range forOriginUserID {
						if err := a.DB.StoreCrossSigningSigsForTarget(
							ctx, originUserID, originKeyID, targetUserID, targetKeyID, originSig,
						); err != nil {
							return fmt.Errorf("a.DB.StoreCrossSigningKeysForTarget: %w", err)
						}
					}
				}

			case *gomatrixserverlib.DeviceKeys:
				for originUserID, forOriginUserID := range sig.Signatures {
					for originKeyID, originSig := range forOriginUserID {
						if err := a.DB.StoreCrossSigningSigsForTarget(
							ctx, originUserID, originKeyID, targetUserID, targetKeyID, originSig,
						); err != nil {
							return fmt.Errorf("a.DB.StoreCrossSigningKeysForTarget: %w", err)
						}
					}
				}

			default:
				return fmt.Errorf("unexpected type assertion")
			}
		}
	}

	return nil
}

func (a *KeyInternalAPI) processOtherSignatures(
	ctx context.Context, userID string,
	signatures map[string]map[gomatrixserverlib.KeyID]gomatrixserverlib.CrossSigningForKeyOrDevice,
) error {
	// Here we will process:
	// * A user signing someone else's master keys using their user-signing keys

	return nil
}

func (a *KeyInternalAPI) crossSigningKeysFromDatabase(
	ctx context.Context, req *api.QueryKeysRequest, res *api.QueryKeysResponse,
) {
	for userID := range req.UserToDevices {
		keys, err := a.DB.CrossSigningKeysForUser(ctx, userID)
		if err != nil {
			logrus.WithError(err).Errorf("Failed to get cross-signing keys for user %q", userID)
			continue
		}

		for keyType, keyData := range keys {
			b64 := keyData.Encode()
			keyID := gomatrixserverlib.KeyID("ed25519:" + b64)
			key := gomatrixserverlib.CrossSigningKey{
				UserID: userID,
				Usage: []gomatrixserverlib.CrossSigningKeyPurpose{
					keyType,
				},
				Keys: map[gomatrixserverlib.KeyID]gomatrixserverlib.Base64Bytes{
					keyID: keyData,
				},
			}

			sigs, err := a.DB.CrossSigningSigsForTarget(ctx, userID, keyID)
			if err != nil {
				logrus.WithError(err).Errorf("Failed to get cross-signing signatures for user %q key %q", userID, keyID)
				continue
			}

			appendSignature := func(originUserID string, originKeyID gomatrixserverlib.KeyID, signature gomatrixserverlib.Base64Bytes) {
				if key.Signatures == nil {
					key.Signatures = types.CrossSigningSigMap{}
				}
				if _, ok := key.Signatures[originUserID]; !ok {
					key.Signatures[originUserID] = make(map[gomatrixserverlib.KeyID]gomatrixserverlib.Base64Bytes)
				}
				key.Signatures[originUserID][originKeyID] = signature
			}

			for originUserID, forOrigin := range sigs {
				for originKeyID, signature := range forOrigin {
					switch {
					case req.UserID != "" && originUserID == req.UserID:
						// Include signatures that we created
						appendSignature(originUserID, originKeyID, signature)
					case originUserID == userID:
						// Include signatures that were created by the person whose key
						// we are processing
						appendSignature(originUserID, originKeyID, signature)
					}
				}
			}

			switch keyType {
			case gomatrixserverlib.CrossSigningKeyPurposeMaster:
				res.MasterKeys[userID] = key

			case gomatrixserverlib.CrossSigningKeyPurposeSelfSigning:
				res.SelfSigningKeys[userID] = key

			case gomatrixserverlib.CrossSigningKeyPurposeUserSigning:
				res.UserSigningKeys[userID] = key
			}
		}
	}
}
