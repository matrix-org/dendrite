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
	"database/sql"
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
		existingKeys, err := a.DB.CrossSigningKeysDataForUser(ctx, req.UserID)
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
		// all of the key IDs specified in the signatures.
		keyJSON, err := json.Marshal(key)
		if err != nil {
			res.Error = &api.KeyError{
				Err: fmt.Sprintf("The JSON of the key section is invalid: %s", err.Error()),
			}
			return
		}

		switch purpose {
		case gomatrixserverlib.CrossSigningKeyPurposeMaster:
			// The master key might have a signature attached to it from the
			// previous key, or from a device key, but there's no real need
			// to verify it. Clients will perform key checks when the master
			// key changes.

		default:
			// Sub-keys should be signed by the master key.
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
		return
	}

	// Now upload any signatures that were included with the keys.
	for _, key := range toVerify {
		var targetKeyID gomatrixserverlib.KeyID
		for targetKey := range key.Keys { // iterates once, see sanityCheckKey
			targetKeyID = targetKey
		}
		for sigUserID, forSigUserID := range key.Signatures {
			if sigUserID != req.UserID {
				continue
			}
			for sigKeyID, sigBytes := range forSigUserID {
				if err := a.DB.StoreCrossSigningSigsForTarget(ctx, sigUserID, sigKeyID, req.UserID, targetKeyID, sigBytes); err != nil {
					res.Error = &api.KeyError{
						Err: fmt.Sprintf("a.DB.StoreCrossSigningSigsForTarget: %s", err),
					}
					return
				}
			}
		}
	}
}

func (a *KeyInternalAPI) PerformUploadDeviceSignatures(ctx context.Context, req *api.PerformUploadDeviceSignaturesRequest, res *api.PerformUploadDeviceSignaturesResponse) {
	// Before we do anything, we need the master and self-signing keys for this user.
	// Then we can verify the signatures make sense.
	queryReq := &api.QueryKeysRequest{
		UserID:        req.UserID,
		UserToDevices: map[string][]string{},
	}
	queryRes := &api.QueryKeysResponse{}
	for userID := range req.Signatures {
		queryReq.UserToDevices[userID] = []string{}
	}
	a.QueryKeys(ctx, queryReq, queryRes)

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

	if err := a.processSelfSignatures(ctx, req.UserID, queryRes, selfSignatures); err != nil {
		res.Error = &api.KeyError{
			Err: fmt.Sprintf("a.processSelfSignatures: %s", err),
		}
		return
	}

	if err := a.processOtherSignatures(ctx, req.UserID, queryRes, otherSignatures); err != nil {
		res.Error = &api.KeyError{
			Err: fmt.Sprintf("a.processOtherSignatures: %s", err),
		}
		return
	}
}

func (a *KeyInternalAPI) processSelfSignatures(
	ctx context.Context, _ string, queryRes *api.QueryKeysResponse,
	signatures map[string]map[gomatrixserverlib.KeyID]gomatrixserverlib.CrossSigningForKeyOrDevice,
) error {
	// Here we will process:
	// * The user signing their own devices using their self-signing key
	// * The user signing their master key using one of their devices

	for targetUserID, forTargetUserID := range signatures {
		for targetKeyID, signature := range forTargetUserID {
			switch sig := signature.CrossSigningBody.(type) {
			case *gomatrixserverlib.CrossSigningKey:
				// The user is signing their master key with one of their devices
				// The QueryKeys response should contain the device key hopefully.
				// First we need to marshal the blob back into JSON so we can verify
				// it.
				j, err := json.Marshal(sig)
				if err != nil {
					return fmt.Errorf("json.Marshal: %w", err)
				}

				for originUserID, forOriginUserID := range sig.Signatures {
					originDeviceKeys, ok := queryRes.DeviceKeys[originUserID]
					if !ok {
						return fmt.Errorf("missing device keys for user %q", originUserID)
					}

					for originKeyID, originSig := range forOriginUserID {
						originDeviceKeyID := gomatrixserverlib.KeyID("ed25519:" + originKeyID)

						var originKey gomatrixserverlib.DeviceKeys
						if err := json.Unmarshal(originDeviceKeys[string(originKeyID)], &originKey); err != nil {
							return fmt.Errorf("json.Unmarshal: %w", err)
						}

						originSigningKey, ok := originKey.Keys[originDeviceKeyID]
						if !ok {
							return fmt.Errorf("missing origin signing key %q", originDeviceKeyID)
						}
						originSigningKeyPublic := ed25519.PublicKey(originSigningKey)

						if err := gomatrixserverlib.VerifyJSON(originUserID, originDeviceKeyID, originSigningKeyPublic, j); err != nil {
							return fmt.Errorf("gomatrixserverlib.VerifyJSON: %w", err)
						}

						if err := a.DB.StoreCrossSigningSigsForTarget(
							ctx, originUserID, originKeyID, targetUserID, targetKeyID, originSig,
						); err != nil {
							return fmt.Errorf("a.DB.StoreCrossSigningKeysForTarget: %w", err)
						}
					}
				}

			case *gomatrixserverlib.DeviceKeys:
				// The user is signing one of their devices with their self-signing key
				// The QueryKeys response should contain the master key hopefully.
				// First we need to marshal the blob back into JSON so we can verify
				// it.
				j, err := json.Marshal(sig)
				if err != nil {
					return fmt.Errorf("json.Marshal: %w", err)
				}

				for originUserID, forOriginUserID := range sig.Signatures {
					for originKeyID, originSig := range forOriginUserID {
						originSelfSigningKeys, ok := queryRes.SelfSigningKeys[originUserID]
						if !ok {
							return fmt.Errorf("missing self-signing key for user %q", originUserID)
						}

						var originSelfSigningKeyID gomatrixserverlib.KeyID
						var originSelfSigningKey gomatrixserverlib.Base64Bytes
						for keyID, key := range originSelfSigningKeys.Keys {
							originSelfSigningKeyID, originSelfSigningKey = keyID, key
							break
						}

						originSelfSigningKeyPublic := ed25519.PublicKey(originSelfSigningKey)

						if err := gomatrixserverlib.VerifyJSON(originUserID, originSelfSigningKeyID, originSelfSigningKeyPublic, j); err != nil {
							return fmt.Errorf("gomatrixserverlib.VerifyJSON: %w", err)
						}

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
	ctx context.Context, userID string, queryRes *api.QueryKeysResponse,
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

		for keyType, key := range keys {
			var keyID gomatrixserverlib.KeyID
			for id := range key.Keys {
				keyID = id
				break
			}

			sigMap, err := a.DB.CrossSigningSigsForTarget(ctx, userID, keyID)
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

			for originUserID, forOrigin := range sigMap {
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

func (a *KeyInternalAPI) QuerySignatures(ctx context.Context, req *api.QuerySignaturesRequest, res *api.QuerySignaturesResponse) {
	for targetUserID, forTargetUser := range req.TargetIDs {
		for _, targetKeyID := range forTargetUser {
			keyMap, err := a.DB.CrossSigningKeysForUser(ctx, targetUserID)
			if err != nil {
				if err == sql.ErrNoRows {
					continue
				}
				res.Error = &api.KeyError{
					Err: fmt.Sprintf("a.DB.CrossSigningKeysForUser: %s", err),
				}
			}

			for targetPurpose, targetKey := range keyMap {
				switch targetPurpose {
				case gomatrixserverlib.CrossSigningKeyPurposeMaster:
					if res.MasterKeys == nil {
						res.MasterKeys = map[string]gomatrixserverlib.CrossSigningKey{}
					}
					res.MasterKeys[targetUserID] = targetKey

				case gomatrixserverlib.CrossSigningKeyPurposeSelfSigning:
					if res.SelfSigningKeys == nil {
						res.SelfSigningKeys = map[string]gomatrixserverlib.CrossSigningKey{}
					}
					res.SelfSigningKeys[targetUserID] = targetKey

				case gomatrixserverlib.CrossSigningKeyPurposeUserSigning:
					if res.UserSigningKeys == nil {
						res.UserSigningKeys = map[string]gomatrixserverlib.CrossSigningKey{}
					}
					res.UserSigningKeys[targetUserID] = targetKey
				}
			}

			sigMap, err := a.DB.CrossSigningSigsForTarget(ctx, targetUserID, targetKeyID)
			if err != nil {
				if err == sql.ErrNoRows {
					continue
				}
				res.Error = &api.KeyError{
					Err: fmt.Sprintf("a.DB.CrossSigningSigsForTarget: %s", err),
				}
				return
			}

			for sourceUserID, forSourceUser := range sigMap {
				for sourceKeyID, sourceSig := range forSourceUser {
					if res.Signatures == nil {
						res.Signatures = map[string]map[gomatrixserverlib.KeyID]types.CrossSigningSigMap{}
					}
					if _, ok := res.Signatures[targetUserID]; !ok {
						res.Signatures[targetUserID] = map[gomatrixserverlib.KeyID]types.CrossSigningSigMap{}
					}
					if _, ok := res.Signatures[targetUserID][targetKeyID]; !ok {
						res.Signatures[targetUserID][targetKeyID] = types.CrossSigningSigMap{}
					}
					if _, ok := res.Signatures[targetUserID][targetKeyID][sourceUserID]; !ok {
						res.Signatures[targetUserID][targetKeyID][sourceUserID] = map[gomatrixserverlib.KeyID]gomatrixserverlib.Base64Bytes{}
					}
					res.Signatures[targetUserID][targetKeyID][sourceUserID][sourceKeyID] = sourceSig
				}
			}
		}
	}
}
