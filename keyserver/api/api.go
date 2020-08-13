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
	"context"
	"encoding/json"
	"strings"
	"time"

	userapi "github.com/matrix-org/dendrite/userapi/api"
	"github.com/matrix-org/gomatrixserverlib"
)

type KeyInternalAPI interface {
	// SetUserAPI assigns a user API to query when extracting device names.
	SetUserAPI(i userapi.UserInternalAPI)
	// InputDeviceListUpdate from a federated server EDU
	InputDeviceListUpdate(ctx context.Context, req *InputDeviceListUpdateRequest, res *InputDeviceListUpdateResponse)
	PerformUploadKeys(ctx context.Context, req *PerformUploadKeysRequest, res *PerformUploadKeysResponse)
	// PerformClaimKeys claims one-time keys for use in pre-key messages
	PerformClaimKeys(ctx context.Context, req *PerformClaimKeysRequest, res *PerformClaimKeysResponse)
	QueryKeys(ctx context.Context, req *QueryKeysRequest, res *QueryKeysResponse)
	QueryKeyChanges(ctx context.Context, req *QueryKeyChangesRequest, res *QueryKeyChangesResponse)
	QueryOneTimeKeys(ctx context.Context, req *QueryOneTimeKeysRequest, res *QueryOneTimeKeysResponse)
	QueryDeviceMessages(ctx context.Context, req *QueryDeviceMessagesRequest, res *QueryDeviceMessagesResponse)
}

// KeyError is returned if there was a problem performing/querying the server
type KeyError struct {
	Err string
}

func (k *KeyError) Error() string {
	return k.Err
}

// DeviceMessage represents the message produced into Kafka by the key server.
type DeviceMessage struct {
	DeviceKeys
	// A monotonically increasing number which represents device changes for this user.
	StreamID int
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
func (k *DeviceKeys) WithStreamID(streamID int) DeviceMessage {
	return DeviceMessage{
		DeviceKeys: *k,
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

type QueryKeysRequest struct {
	// Maps user IDs to a list of devices
	UserToDevices map[string][]string
	Timeout       time.Duration
}

type QueryKeysResponse struct {
	// Map of remote server domain to error JSON
	Failures map[string]interface{}
	// Map of user_id to device_id to device_key
	DeviceKeys map[string]map[string]json.RawMessage
	// Set if there was a fatal error processing this query
	Error *KeyError
}

type QueryKeyChangesRequest struct {
	// The partition which had key events sent to
	Partition int32
	// The offset of the last received key event, or sarama.OffsetOldest if this is from the beginning
	Offset int64
	// The inclusive offset where to track key changes up to. Messages with this offset are included in the response.
	// Use sarama.OffsetNewest if the offset is unknown (then check the response Offset to avoid racing).
	ToOffset int64
}

type QueryKeyChangesResponse struct {
	// The set of users who have had their keys change.
	UserIDs []string
	// The partition being served - useful if the partition is unknown at request time
	Partition int32
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
	StreamID int
	Devices  []DeviceMessage
	Error    *KeyError
}

type InputDeviceListUpdateRequest struct {
	Event gomatrixserverlib.DeviceListUpdateEvent
}

type InputDeviceListUpdateResponse struct {
	Error *KeyError
}
