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

package api

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"time"

	"github.com/matrix-org/gomatrixserverlib"

	"github.com/matrix-org/dendrite/keyserver/types"
	userapi "github.com/matrix-org/dendrite/userapi/api"
)

type KeyInternalAPI interface {
	SyncKeyAPI
	ClientKeyAPI
	FederationKeyAPI
	UserKeyAPI

	// SetUserAPI assigns a user API to query when extracting device names.
	SetUserAPI(i userapi.KeyserverUserAPI)
}

// API functions required by the clientapi
type ClientKeyAPI interface {
	QueryKeys(ctx context.Context, req *QueryKeysRequest, res *QueryKeysResponse) error
	PerformUploadKeys(ctx context.Context, req *PerformUploadKeysRequest, res *PerformUploadKeysResponse) error
	PerformUploadDeviceKeys(ctx context.Context, req *PerformUploadDeviceKeysRequest, res *PerformUploadDeviceKeysResponse) error
	PerformUploadDeviceSignatures(ctx context.Context, req *PerformUploadDeviceSignaturesRequest, res *PerformUploadDeviceSignaturesResponse) error
	// PerformClaimKeys claims one-time keys for use in pre-key messages
	PerformClaimKeys(ctx context.Context, req *PerformClaimKeysRequest, res *PerformClaimKeysResponse) error
	PerformMarkAsStaleIfNeeded(ctx context.Context, req *PerformMarkAsStaleRequest, res *struct{}) error
}

// API functions required by the userapi
type UserKeyAPI interface {
	PerformUploadKeys(ctx context.Context, req *PerformUploadKeysRequest, res *PerformUploadKeysResponse) error
	PerformDeleteKeys(ctx context.Context, req *PerformDeleteKeysRequest, res *PerformDeleteKeysResponse) error
}

// API functions required by the syncapi
type SyncKeyAPI interface {
	QueryKeyChanges(ctx context.Context, req *QueryKeyChangesRequest, res *QueryKeyChangesResponse) error
	QueryOneTimeKeys(ctx context.Context, req *QueryOneTimeKeysRequest, res *QueryOneTimeKeysResponse) error
	PerformMarkAsStaleIfNeeded(ctx context.Context, req *PerformMarkAsStaleRequest, res *struct{}) error
}

type FederationKeyAPI interface {
	QueryKeys(ctx context.Context, req *QueryKeysRequest, res *QueryKeysResponse) error
	QuerySignatures(ctx context.Context, req *QuerySignaturesRequest, res *QuerySignaturesResponse) error
	QueryDeviceMessages(ctx context.Context, req *QueryDeviceMessagesRequest, res *QueryDeviceMessagesResponse) error
	PerformUploadDeviceKeys(ctx context.Context, req *PerformUploadDeviceKeysRequest, res *PerformUploadDeviceKeysResponse) error
	PerformClaimKeys(ctx context.Context, req *PerformClaimKeysRequest, res *PerformClaimKeysResponse) error
}

// KeyError is returned if there was a problem performing/querying the server
type KeyError struct {
	Err                string `json:"error"`
	IsInvalidSignature bool   `json:"is_invalid_signature,omitempty"` // M_INVALID_SIGNATURE
	IsMissingParam     bool   `json:"is_missing_param,omitempty"`     // M_MISSING_PARAM
	IsInvalidParam     bool   `json:"is_invalid_param,omitempty"`     // M_INVALID_PARAM
}

func (k *KeyError) Error() string {
	return k.Err
}

type DeviceMessageType int

const (
	TypeDeviceKeyUpdate DeviceMessageType = iota
	TypeCrossSigningUpdate
)

// DeviceMessage represents the message produced into Kafka by the key server.
type DeviceMessage struct {
	Type                         DeviceMessageType `json:"Type,omitempty"`
	*DeviceKeys                  `json:"DeviceKeys,omitempty"`
	*OutputCrossSigningKeyUpdate `json:"CrossSigningKeyUpdate,omitempty"`
	// A monotonically increasing number which represents device changes for this user.
	StreamID       int64
	DeviceChangeID int64
}

// OutputCrossSigningKeyUpdate is an entry in the signing key update output kafka log
type OutputCrossSigningKeyUpdate struct {
	CrossSigningKeyUpdate `json:"signing_keys"`
}

type CrossSigningKeyUpdate struct {
	MasterKey      *gomatrixserverlib.CrossSigningKey `json:"master_key,omitempty"`
	SelfSigningKey *gomatrixserverlib.CrossSigningKey `json:"self_signing_key,omitempty"`
	UserID         string                             `json:"user_id"`
}

// DeviceKeysEqual returns true if the device keys updates contain the
// same display name and key JSON. This will return false if either of
// the updates is not a device keys update, or if the user ID/device ID
// differ between the two.
func (m1 *DeviceMessage) DeviceKeysEqual(m2 *DeviceMessage) bool {
	if m1.DeviceKeys == nil || m2.DeviceKeys == nil {
		return false
	}
	if m1.UserID != m2.UserID || m1.DeviceID != m2.DeviceID {
		return false
	}
	if m1.DisplayName != m2.DisplayName {
		return false // different display names
	}
	if len(m1.KeyJSON) == 0 || len(m2.KeyJSON) == 0 {
		return false // either is empty
	}
	return bytes.Equal(m1.KeyJSON, m2.KeyJSON)
}

// DeviceKeys represents a set of device keys for a single device
// https://matrix.org/docs/spec/client_server/r0.6.1#post-matrix-client-r0-keys-upload
type DeviceKeys struct {
	// The user who owns this device
	UserID string
	// The device ID of this device
	DeviceID string
	// The device display name
	DisplayName string
	// The raw device key JSON
	KeyJSON []byte
}

// WithStreamID returns a copy of this device message with the given stream ID
func (k *DeviceKeys) WithStreamID(streamID int64) DeviceMessage {
	return DeviceMessage{
		DeviceKeys: k,
		StreamID:   streamID,
	}
}

// OneTimeKeys represents a set of one-time keys for a single device
// https://matrix.org/docs/spec/client_server/r0.6.1#post-matrix-client-r0-keys-upload
type OneTimeKeys struct {
	// The user who owns this device
	UserID string
	// The device ID of this device
	DeviceID string
	// A map of algorithm:key_id => key JSON
	KeyJSON map[string]json.RawMessage
}

// Split a key in KeyJSON into algorithm and key ID
func (k *OneTimeKeys) Split(keyIDWithAlgo string) (algo string, keyID string) {
	segments := strings.Split(keyIDWithAlgo, ":")
	return segments[0], segments[1]
}

// OneTimeKeysCount represents the counts of one-time keys for a single device
type OneTimeKeysCount struct {
	// The user who owns this device
	UserID string
	// The device ID of this device
	DeviceID string
	// algorithm to count e.g:
	// {
	//   "curve25519": 10,
	//   "signed_curve25519": 20
	// }
	KeyCount map[string]int
}

// PerformUploadKeysRequest is the request to PerformUploadKeys
type PerformUploadKeysRequest struct {
	UserID      string // Required - User performing the request
	DeviceID    string // Optional - Device performing the request, for fetching OTK count
	DeviceKeys  []DeviceKeys
	OneTimeKeys []OneTimeKeys
	// OnlyDisplayNameUpdates should be `true` if ALL the DeviceKeys are present to update
	// the display name for their respective device, and NOT to modify the keys. The key
	// itself doesn't change but it's easier to pretend upload new keys and reuse the same code paths.
	// Without this flag, requests to modify device display names would delete device keys.
	OnlyDisplayNameUpdates bool
}

// PerformUploadKeysResponse is the response to PerformUploadKeys
type PerformUploadKeysResponse struct {
	// A fatal error when processing e.g database failures
	Error *KeyError
	// A map of user_id -> device_id -> Error for tracking failures.
	KeyErrors        map[string]map[string]*KeyError
	OneTimeKeyCounts []OneTimeKeysCount
}

// PerformDeleteKeysRequest asks the keyserver to forget about certain
// keys, and signatures related to those keys.
type PerformDeleteKeysRequest struct {
	UserID string
	KeyIDs []gomatrixserverlib.KeyID
}

// PerformDeleteKeysResponse is the response to PerformDeleteKeysRequest.
type PerformDeleteKeysResponse struct {
	Error *KeyError
}

// KeyError sets a key error field on KeyErrors
func (r *PerformUploadKeysResponse) KeyError(userID, deviceID string, err *KeyError) {
	if r.KeyErrors[userID] == nil {
		r.KeyErrors[userID] = make(map[string]*KeyError)
	}
	r.KeyErrors[userID][deviceID] = err
}

type PerformClaimKeysRequest struct {
	// Map of user_id to device_id to algorithm name
	OneTimeKeys map[string]map[string]string
	Timeout     time.Duration
}

type PerformClaimKeysResponse struct {
	// Map of user_id to device_id to algorithm:key_id to key JSON
	OneTimeKeys map[string]map[string]map[string]json.RawMessage
	// Map of remote server domain to error JSON
	Failures map[string]interface{}
	// Set if there was a fatal error processing this action
	Error *KeyError
}

type PerformUploadDeviceKeysRequest struct {
	gomatrixserverlib.CrossSigningKeys
	// The user that uploaded the key, should be populated by the clientapi.
	UserID string
}

type PerformUploadDeviceKeysResponse struct {
	Error *KeyError
}

type PerformUploadDeviceSignaturesRequest struct {
	Signatures map[string]map[gomatrixserverlib.KeyID]gomatrixserverlib.CrossSigningForKeyOrDevice
	// The user that uploaded the sig, should be populated by the clientapi.
	UserID string
}

type PerformUploadDeviceSignaturesResponse struct {
	Error *KeyError
}

type QueryKeysRequest struct {
	// The user ID asking for the keys, e.g. if from a client API request.
	// Will not be populated if the key request came from federation.
	UserID string
	// Maps user IDs to a list of devices
	UserToDevices map[string][]string
	Timeout       time.Duration
}

type QueryKeysResponse struct {
	// Map of remote server domain to error JSON
	Failures map[string]interface{}
	// Map of user_id to device_id to device_key
	DeviceKeys map[string]map[string]json.RawMessage
	// Maps of user_id to cross signing key
	MasterKeys      map[string]gomatrixserverlib.CrossSigningKey
	SelfSigningKeys map[string]gomatrixserverlib.CrossSigningKey
	UserSigningKeys map[string]gomatrixserverlib.CrossSigningKey
	// Set if there was a fatal error processing this query
	Error *KeyError
}

type QueryKeyChangesRequest struct {
	// The offset of the last received key event, or sarama.OffsetOldest if this is from the beginning
	Offset int64
	// The inclusive offset where to track key changes up to. Messages with this offset are included in the response.
	// Use types.OffsetNewest if the offset is unknown (then check the response Offset to avoid racing).
	ToOffset int64
}

type QueryKeyChangesResponse struct {
	// The set of users who have had their keys change.
	UserIDs []string
	// The latest offset represented in this response.
	Offset int64
	// Set if there was a problem handling the request.
	Error *KeyError
}

type QueryOneTimeKeysRequest struct {
	// The local user to query OTK counts for
	UserID string
	// The device to query OTK counts for
	DeviceID string
}

type QueryOneTimeKeysResponse struct {
	// OTK key counts, in the extended /sync form described by https://matrix.org/docs/spec/client_server/r0.6.1#id84
	Count OneTimeKeysCount
	Error *KeyError
}

type QueryDeviceMessagesRequest struct {
	UserID string
}

type QueryDeviceMessagesResponse struct {
	// The latest stream ID
	StreamID int64
	Devices  []DeviceMessage
	Error    *KeyError
}

type QuerySignaturesRequest struct {
	// A map of target user ID -> target key/device IDs to retrieve signatures for
	TargetIDs map[string][]gomatrixserverlib.KeyID `json:"target_ids"`
}

type QuerySignaturesResponse struct {
	// A map of target user ID -> target key/device ID -> origin user ID -> origin key/device ID -> signatures
	Signatures map[string]map[gomatrixserverlib.KeyID]types.CrossSigningSigMap
	// A map of target user ID -> cross-signing master key
	MasterKeys map[string]gomatrixserverlib.CrossSigningKey
	// A map of target user ID -> cross-signing self-signing key
	SelfSigningKeys map[string]gomatrixserverlib.CrossSigningKey
	// A map of target user ID -> cross-signing user-signing key
	UserSigningKeys map[string]gomatrixserverlib.CrossSigningKey
	// The request error, if any
	Error *KeyError
}

type PerformMarkAsStaleRequest struct {
	UserID   string
	Domain   gomatrixserverlib.ServerName
	DeviceID string
}
