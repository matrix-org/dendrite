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
	"bytes"
	"context"
	"crypto/ed25519"
	"database/sql"
	"fmt"
	"strings"

	"github.com/matrix-org/dendrite/keyserver/api"
	"github.com/matrix-org/dendrite/keyserver/types"
	"github.com/matrix-org/gomatrixserverlib"
	"github.com/sirupsen/logrus"
	"golang.org/x/crypto/curve25519"
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
		switch tokens[0] {
		case "ed25519":
			if len(keyData) != ed25519.PublicKeySize {
				return fmt.Errorf("ed25519 key is not the correct length")
			}
		case "curve25519":
			if len(keyData) != curve25519.PointSize {
				return fmt.Errorf("curve25519 key is not the correct length")
			}
		default:
			// We can't enforce the key length to be correct for an
			// algorithm that we don't recognise, so instead we'll
			// just make sure that it isn't incredibly excessive.
			if l := len(keyData); l > 4096 {
				return fmt.Errorf("unknown key type is too long (%d bytes)", l)
			}
		}
	}

	// Check to see if the signatures make sense
	for _, forOriginUser := range key.Signatures {
		for originKeyID, originSignature := range forOriginUser {
			switch strings.SplitN(string(originKeyID), ":", 1)[0] {
			case "ed25519":
				if len(originSignature) != ed25519.SignatureSize {
					return fmt.Errorf("ed25519 signature is not the correct length")
				}
			case "curve25519":
				return fmt.Errorf("curve25519 signatures are impossible")
			default:
				if l := len(originSignature); l > 4096 {
					return fmt.Errorf("unknown signature type is too long (%d bytes)", l)
				}
			}
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
func (a *KeyInternalAPI) PerformUploadDeviceKeys(ctx context.Context, req *api.PerformUploadDeviceKeysRequest, res *api.PerformUploadDeviceKeysResponse) error {
	// Find the keys to store.
	byPurpose := map[gomatrixserverlib.CrossSigningKeyPurpose]gomatrixserverlib.CrossSigningKey{}
	toStore := types.CrossSigningKeyMap{}
	hasMasterKey := false

	if len(req.MasterKey.Keys) > 0 {
		if err := sanityCheckKey(req.MasterKey, req.UserID, gomatrixserverlib.CrossSigningKeyPurposeMaster); err != nil {
			res.Error = &api.KeyError{
				Err:            "Master key sanity check failed: " + err.Error(),
				IsInvalidParam: true,
			}
			return nil
		}

		byPurpose[gomatrixserverlib.CrossSigningKeyPurposeMaster] = req.MasterKey
		for _, key := range req.MasterKey.Keys { // iterates once, see sanityCheckKey
			toStore[gomatrixserverlib.CrossSigningKeyPurposeMaster] = key
		}
		hasMasterKey = true
	}

	if len(req.SelfSigningKey.Keys) > 0 {
		if err := sanityCheckKey(req.SelfSigningKey, req.UserID, gomatrixserverlib.CrossSigningKeyPurposeSelfSigning); err != nil {
			res.Error = &api.KeyError{
				Err:            "Self-signing key sanity check failed: " + err.Error(),
				IsInvalidParam: true,
			}
			return nil
		}

		byPurpose[gomatrixserverlib.CrossSigningKeyPurposeSelfSigning] = req.SelfSigningKey
		for _, key := range req.SelfSigningKey.Keys { // iterates once, see sanityCheckKey
			toStore[gomatrixserverlib.CrossSigningKeyPurposeSelfSigning] = key
		}
	}

	if len(req.UserSigningKey.Keys) > 0 {
		if err := sanityCheckKey(req.UserSigningKey, req.UserID, gomatrixserverlib.CrossSigningKeyPurposeUserSigning); err != nil {
			res.Error = &api.KeyError{
				Err:            "User-signing key sanity check failed: " + err.Error(),
				IsInvalidParam: true,
			}
			return nil
		}

		byPurpose[gomatrixserverlib.CrossSigningKeyPurposeUserSigning] = req.UserSigningKey
		for _, key := range req.UserSigningKey.Keys { // iterates once, see sanityCheckKey
			toStore[gomatrixserverlib.CrossSigningKeyPurposeUserSigning] = key
		}
	}

	// If there's nothing to do then stop here.
	if len(toStore) == 0 {
		res.Error = &api.KeyError{
			Err:            "No keys were supplied in the request",
			IsMissingParam: true,
		}
		return nil
	}

	// We can't have a self-signing or user-signing key without a master
	// key, so make sure we have one of those. We will also only actually do
	// something if any of the specified keys in the request are different
	// to what we've got in the database, to avoid generating key change
	// notifications unnecessarily.
	existingKeys, err := a.DB.CrossSigningKeysDataForUser(ctx, req.UserID)
	if err != nil {
		res.Error = &api.KeyError{
			Err: "Retrieving cross-signing keys from database failed: " + err.Error(),
		}
		return nil
	}

	// If we still can't find a master key for the user then stop the upload.
	// This satisfies the "Fails to upload self-signing key without master key" test.
	if !hasMasterKey {
		if _, hasMasterKey = existingKeys[gomatrixserverlib.CrossSigningKeyPurposeMaster]; !hasMasterKey {
			res.Error = &api.KeyError{
				Err:            "No master key was found",
				IsMissingParam: true,
			}
			return nil
		}
	}

	// Check if anything actually changed compared to what we have in the database.
	changed := false
	for _, purpose := range []gomatrixserverlib.CrossSigningKeyPurpose{
		gomatrixserverlib.CrossSigningKeyPurposeMaster,
		gomatrixserverlib.CrossSigningKeyPurposeSelfSigning,
		gomatrixserverlib.CrossSigningKeyPurposeUserSigning,
	} {
		old, gotOld := existingKeys[purpose]
		new, gotNew := toStore[purpose]
		if gotOld != gotNew {
			// A new key purpose has been specified that we didn't know before,
			// or one has been removed.
			changed = true
			break
		}
		if !bytes.Equal(old, new) {
			// One of the existing keys for a purpose we already knew about has
			// changed.
			changed = true
			break
		}
	}
	if !changed {
		return nil
	}

	// Store the keys.
	if err := a.DB.StoreCrossSigningKeysForUser(ctx, req.UserID, toStore); err != nil {
		res.Error = &api.KeyError{
			Err: fmt.Sprintf("a.DB.StoreCrossSigningKeysForUser: %s", err),
		}
		return nil
	}

	// Now upload any signatures that were included with the keys.
	for _, key := range byPurpose {
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
					return nil
				}
			}
		}
	}

	// Finally, generate a notification that we updated the keys.
	update := api.CrossSigningKeyUpdate{
		UserID: req.UserID,
	}
	if mk, ok := byPurpose[gomatrixserverlib.CrossSigningKeyPurposeMaster]; ok {
		update.MasterKey = &mk
	}
	if ssk, ok := byPurpose[gomatrixserverlib.CrossSigningKeyPurposeSelfSigning]; ok {
		update.SelfSigningKey = &ssk
	}
	if update.MasterKey == nil && update.SelfSigningKey == nil {
		return nil
	}
	if err := a.Producer.ProduceSigningKeyUpdate(update); err != nil {
		res.Error = &api.KeyError{
			Err: fmt.Sprintf("a.Producer.ProduceSigningKeyUpdate: %s", err),
		}
		return nil
	}
	return nil
}

func (a *KeyInternalAPI) PerformUploadDeviceSignatures(ctx context.Context, req *api.PerformUploadDeviceSignaturesRequest, res *api.PerformUploadDeviceSignaturesResponse) error {
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
	_ = a.QueryKeys(ctx, queryReq, queryRes)

	selfSignatures := map[string]map[gomatrixserverlib.KeyID]gomatrixserverlib.CrossSigningForKeyOrDevice{}
	otherSignatures := map[string]map[gomatrixserverlib.KeyID]gomatrixserverlib.CrossSigningForKeyOrDevice{}

	// Sort signatures into two groups: one where people have signed their own
	// keys and one where people have signed someone elses
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

	if err := a.processSelfSignatures(ctx, selfSignatures); err != nil {
		res.Error = &api.KeyError{
			Err: fmt.Sprintf("a.processSelfSignatures: %s", err),
		}
		return nil
	}

	if err := a.processOtherSignatures(ctx, req.UserID, queryRes, otherSignatures); err != nil {
		res.Error = &api.KeyError{
			Err: fmt.Sprintf("a.processOtherSignatures: %s", err),
		}
		return nil
	}

	// Finally, generate a notification that we updated the signatures.
	for userID := range req.Signatures {
		masterKey := queryRes.MasterKeys[userID]
		selfSigningKey := queryRes.SelfSigningKeys[userID]
		update := api.CrossSigningKeyUpdate{
			UserID:         userID,
			MasterKey:      &masterKey,
			SelfSigningKey: &selfSigningKey,
		}
		if err := a.Producer.ProduceSigningKeyUpdate(update); err != nil {
			res.Error = &api.KeyError{
				Err: fmt.Sprintf("a.Producer.ProduceSigningKeyUpdate: %s", err),
			}
			return nil
		}
	}
	return nil
}

func (a *KeyInternalAPI) processSelfSignatures(
	ctx context.Context,
	signatures map[string]map[gomatrixserverlib.KeyID]gomatrixserverlib.CrossSigningForKeyOrDevice,
) error {
	// Here we will process:
	// * The user signing their own devices using their self-signing key
	// * The user signing their master key using one of their devices

	for targetUserID, forTargetUserID := range signatures {
		for targetKeyID, signature := range forTargetUserID {
			switch sig := signature.CrossSigningBody.(type) {
			case *gomatrixserverlib.CrossSigningKey:
				for keyID := range sig.Keys {
					split := strings.SplitN(string(keyID), ":", 2)
					if len(split) > 1 && gomatrixserverlib.KeyID(split[1]) == targetKeyID {
						targetKeyID = keyID // contains the ed25519: or other scheme
						break
					}
				}
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
	ctx context.Context, userID string, queryRes *api.QueryKeysResponse,
	signatures map[string]map[gomatrixserverlib.KeyID]gomatrixserverlib.CrossSigningForKeyOrDevice,
) error {
	// Here we will process:
	// * A user signing someone else's master keys using their user-signing keys

	for targetUserID, forTargetUserID := range signatures {
		for _, signature := range forTargetUserID {
			switch sig := signature.CrossSigningBody.(type) {
			case *gomatrixserverlib.CrossSigningKey:
				// Find the local copy of the master key. We'll use this to be
				// sure that the supplied stanza matches the key that we think it
				// should be.
				masterKey, ok := queryRes.MasterKeys[targetUserID]
				if !ok {
					return fmt.Errorf("failed to find master key for user %q", targetUserID)
				}

				// For each key ID, write the signatures. Maybe there'll be more
				// than one algorithm in the future so it's best not to focus on
				// everything being ed25519:.
				for targetKeyID, suppliedKeyData := range sig.Keys {
					// The master key will be supplied in the request, but we should
					// make sure that it matches what we think the master key should
					// actually be.
					localKeyData, lok := masterKey.Keys[targetKeyID]
					if !lok {
						return fmt.Errorf("uploaded master key %q for user %q doesn't match local copy", targetKeyID, targetUserID)
					} else if !bytes.Equal(suppliedKeyData, localKeyData) {
						return fmt.Errorf("uploaded master key %q for user %q doesn't match local copy", targetKeyID, targetUserID)
					}

					// We only care about the signatures from the uploading user, so
					// we will ignore anything that didn't originate from them.
					userSigs, ok := sig.Signatures[userID]
					if !ok {
						return fmt.Errorf("there are no signatures on master key %q from uploading user %q", targetKeyID, userID)
					}

					for originKeyID, originSig := range userSigs {
						if err := a.DB.StoreCrossSigningSigsForTarget(
							ctx, userID, originKeyID, targetUserID, targetKeyID, originSig,
						); err != nil {
							return fmt.Errorf("a.DB.StoreCrossSigningKeysForTarget: %w", err)
						}
					}
				}

			default:
				// Users should only be signing another person's master key,
				// so if we're here, it's probably because it's actually a
				// gomatrixserverlib.DeviceKeys, which doesn't make sense.
			}
		}
	}

	return nil
}

func (a *KeyInternalAPI) crossSigningKeysFromDatabase(
	ctx context.Context, req *api.QueryKeysRequest, res *api.QueryKeysResponse,
) {
	for targetUserID := range req.UserToDevices {
		keys, err := a.DB.CrossSigningKeysForUser(ctx, targetUserID)
		if err != nil {
			logrus.WithError(err).Errorf("Failed to get cross-signing keys for user %q", targetUserID)
			continue
		}

		for keyType, key := range keys {
			var keyID gomatrixserverlib.KeyID
			for id := range key.Keys {
				keyID = id
				break
			}

			sigMap, err := a.DB.CrossSigningSigsForTarget(ctx, req.UserID, targetUserID, keyID)
			if err != nil && err != sql.ErrNoRows {
				logrus.WithError(err).Errorf("Failed to get cross-signing signatures for user %q key %q", targetUserID, keyID)
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
					case originUserID == targetUserID:
						// Include signatures that were created by the person whose key
						// we are processing
						appendSignature(originUserID, originKeyID, signature)
					}
				}
			}

			switch keyType {
			case gomatrixserverlib.CrossSigningKeyPurposeMaster:
				res.MasterKeys[targetUserID] = key

			case gomatrixserverlib.CrossSigningKeyPurposeSelfSigning:
				res.SelfSigningKeys[targetUserID] = key

			case gomatrixserverlib.CrossSigningKeyPurposeUserSigning:
				res.UserSigningKeys[targetUserID] = key
			}
		}
	}
}

func (a *KeyInternalAPI) QuerySignatures(ctx context.Context, req *api.QuerySignaturesRequest, res *api.QuerySignaturesResponse) error {
	for targetUserID, forTargetUser := range req.TargetIDs {
		keyMap, err := a.DB.CrossSigningKeysForUser(ctx, targetUserID)
		if err != nil && err != sql.ErrNoRows {
			res.Error = &api.KeyError{
				Err: fmt.Sprintf("a.DB.CrossSigningKeysForUser: %s", err),
			}
			continue
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

		for _, targetKeyID := range forTargetUser {
			// Get own signatures only.
			sigMap, err := a.DB.CrossSigningSigsForTarget(ctx, targetUserID, targetUserID, targetKeyID)
			if err != nil && err != sql.ErrNoRows {
				res.Error = &api.KeyError{
					Err: fmt.Sprintf("a.DB.CrossSigningSigsForTarget: %s", err),
				}
				return nil
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
	return nil
}
