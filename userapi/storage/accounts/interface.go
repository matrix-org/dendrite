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

package accounts

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/matrix-org/dendrite/clientapi/auth/authtypes"
	"github.com/matrix-org/dendrite/internal"
	"github.com/matrix-org/dendrite/userapi/api"
	"github.com/matrix-org/gomatrixserverlib"
)

type Database interface {
	internal.PartitionStorer
	GetAccountByPassword(ctx context.Context, localpart, plaintextPassword string) (*api.Account, error)
	GetProfileByLocalpart(ctx context.Context, localpart string) (*authtypes.Profile, error)
	SetAvatarURL(ctx context.Context, localpart string, avatarURL string) error
	SetDisplayName(ctx context.Context, localpart string, displayName string) error
	// CreateAccount makes a new account with the given login name and password, and creates an empty profile
	// for this account. If no password is supplied, the account will be a passwordless account. If the
	// account already exists, it will return nil, ErrUserExists.
	CreateAccount(ctx context.Context, localpart, plaintextPassword, appserviceID string) (*api.Account, error)
	CreateGuestAccount(ctx context.Context) (*api.Account, error)
	UpdateMemberships(ctx context.Context, eventsToAdd []gomatrixserverlib.Event, idsToRemove []string) error
	GetMembershipInRoomByLocalpart(ctx context.Context, localpart, roomID string) (authtypes.Membership, error)
	GetRoomIDsByLocalPart(ctx context.Context, localpart string) ([]string, error)
	GetMembershipsByLocalpart(ctx context.Context, localpart string) (memberships []authtypes.Membership, err error)
	SaveAccountData(ctx context.Context, localpart, roomID, dataType string, content json.RawMessage) error
	GetAccountData(ctx context.Context, localpart string) (global map[string]json.RawMessage, rooms map[string]map[string]json.RawMessage, err error)
	// GetAccountDataByType returns account data matching a given
	// localpart, room ID and type.
	// If no account data could be found, returns nil
	// Returns an error if there was an issue with the retrieval
	GetAccountDataByType(ctx context.Context, localpart, roomID, dataType string) (data json.RawMessage, err error)
	GetNewNumericLocalpart(ctx context.Context) (int64, error)
	SaveThreePIDAssociation(ctx context.Context, threepid, localpart, medium string) (err error)
	RemoveThreePIDAssociation(ctx context.Context, threepid string, medium string) (err error)
	GetLocalpartForThreePID(ctx context.Context, threepid string, medium string) (localpart string, err error)
	GetThreePIDsForLocalpart(ctx context.Context, localpart string) (threepids []authtypes.ThreePID, err error)
	CheckAccountAvailability(ctx context.Context, localpart string) (bool, error)
	GetAccountByLocalpart(ctx context.Context, localpart string) (*api.Account, error)
}

// Err3PIDInUse is the error returned when trying to save an association involving
// a third-party identifier which is already associated to a local user.
var Err3PIDInUse = errors.New("This third-party identifier is already in use")
