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

package presence

import (
	"context"

	"github.com/matrix-org/dendrite/eduserver/api"
	"github.com/matrix-org/dendrite/userapi/types"
)

type Database interface {
	// UpsertPresence creates/updates the presence status of a user.
	UpsertPresence(
		ctx context.Context,
		userID string,
		statusMsg *string,
		presence types.PresenceStatus,
		lastActiveTS int64,
	) (pos int64, err error)
	// GetPresenceForUser gets the presence status of a user.
	GetPresenceForUser(
		ctx context.Context,
		userID string,
	) (presence api.OutputPresenceData, err error)
	GetPresenceAfter(
		ctx context.Context,
		pos int64,
	) (presence []api.OutputPresenceData, err error)
	GetMaxPresenceID(ctx context.Context) (pos int64, err error)
}
