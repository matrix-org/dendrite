// Copyright 2022 The Matrix.org Foundation C.I.C.
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

package streams

import (
	"context"
	"database/sql"

	"github.com/matrix-org/dendrite/syncapi/notifier"
	"github.com/matrix-org/dendrite/syncapi/storage"
	"github.com/matrix-org/dendrite/syncapi/storage/mrd"
	"github.com/matrix-org/dendrite/syncapi/types"
)

type MultiRoomDataStreamProvider struct {
	DefaultStreamProvider
	notifier *notifier.Notifier
	mrdDb    *mrd.Queries
}

func (p *MultiRoomDataStreamProvider) Setup(ctx context.Context, snapshot storage.DatabaseTransaction) {
	p.DefaultStreamProvider.Setup(ctx, snapshot)

	id, err := p.mrdDb.SelectMaxId(context.Background())
	if err != nil && err != sql.ErrNoRows {
		panic(err)
	}
	p.latestMutex.Lock()
	defer p.latestMutex.Unlock()
	if id == nil {
		p.latest = types.StreamPosition(0)
	} else {
		p.latest = types.StreamPosition(id.(int64))
	}
}

func (p *MultiRoomDataStreamProvider) CompleteSync(
	ctx context.Context,
	snapshot storage.DatabaseTransaction,
	req *types.SyncRequest,
) types.StreamPosition {
	return p.IncrementalSync(ctx, snapshot, req, 0, p.LatestPosition(ctx))
}

func (p *MultiRoomDataStreamProvider) IncrementalSync(
	ctx context.Context,
	snapshot storage.DatabaseTransaction,
	req *types.SyncRequest,
	from, to types.StreamPosition,
) types.StreamPosition {
	mr, err := snapshot.SelectMultiRoomData(ctx, &types.Range{From: from, To: to}, req.JoinedRooms)
	if err != nil {
		req.Log.WithError(err).Error("GetUserUnreadNotificationCountsForRooms failed")
		return from
	}
	req.Log.Tracef("MultiRoomDataStreamProvider IncrementalSync: %+v", mr)
	req.Response.MultiRoom = mr
	return to

}
