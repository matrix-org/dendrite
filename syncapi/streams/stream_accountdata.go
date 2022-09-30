package streams

import (
	"context"

	"github.com/matrix-org/gomatrixserverlib"

	"github.com/matrix-org/dendrite/syncapi/storage"
	"github.com/matrix-org/dendrite/syncapi/types"
	userapi "github.com/matrix-org/dendrite/userapi/api"
)

type AccountDataStreamProvider struct {
	DefaultStreamProvider
	userAPI userapi.SyncUserAPI
}

func (p *AccountDataStreamProvider) Setup(
	ctx context.Context, snapshot storage.DatabaseSnapshot,
) {
	p.DefaultStreamProvider.Setup(ctx, snapshot)

	p.latestMutex.Lock()
	defer p.latestMutex.Unlock()

	p.latest = p.latestPosition(ctx, snapshot)
}

func (p *AccountDataStreamProvider) latestPosition(
	ctx context.Context, snapshot storage.DatabaseSnapshot,
) types.StreamPosition {
	id, err := snapshot.MaxStreamPositionForAccountData(context.Background())
	if err != nil {
		panic(err)
	}
	return id
}

func (p *AccountDataStreamProvider) CompleteSync(
	ctx context.Context,
	snapshot storage.DatabaseSnapshot,
	req *types.SyncRequest,
) types.StreamPosition {
	return p.IncrementalSync(ctx, snapshot, req, 0, p.latestPosition(ctx, snapshot))
}

func (p *AccountDataStreamProvider) IncrementalSync(
	ctx context.Context,
	snapshot storage.DatabaseSnapshot,
	req *types.SyncRequest,
	from, to types.StreamPosition,
) types.StreamPosition {
	r := types.Range{
		From: from,
		To:   to,
	}

	dataTypes, pos, err := snapshot.GetAccountDataInRange(
		ctx, req.Device.UserID, r, &req.Filter.AccountData,
	)
	if err != nil {
		req.Log.WithError(err).Error("p.DB.GetAccountDataInRange failed")
		return from
	}

	// Iterate over the rooms
	for roomID, dataTypes := range dataTypes {
		// For a complete sync, make sure we're only including this room if
		// that room was present in the joined rooms.
		if from == 0 && roomID != "" && !req.IsRoomPresent(roomID) {
			continue
		}

		// Request the missing data from the database
		for _, dataType := range dataTypes {
			dataReq := userapi.QueryAccountDataRequest{
				UserID:   req.Device.UserID,
				RoomID:   roomID,
				DataType: dataType,
			}
			dataRes := userapi.QueryAccountDataResponse{}
			err = p.userAPI.QueryAccountData(ctx, &dataReq, &dataRes)
			if err != nil {
				req.Log.WithError(err).Error("p.userAPI.QueryAccountData failed")
				continue
			}
			if roomID == "" {
				if globalData, ok := dataRes.GlobalAccountData[dataType]; ok {
					req.Response.AccountData.Events = append(
						req.Response.AccountData.Events,
						gomatrixserverlib.ClientEvent{
							Type:    dataType,
							Content: gomatrixserverlib.RawJSON(globalData),
						},
					)
				}
			} else {
				if roomData, ok := dataRes.RoomAccountData[roomID][dataType]; ok {
					joinData := *types.NewJoinResponse()
					if existing, ok := req.Response.Rooms.Join[roomID]; ok {
						joinData = existing
					}
					joinData.AccountData.Events = append(
						joinData.AccountData.Events,
						gomatrixserverlib.ClientEvent{
							Type:    dataType,
							Content: gomatrixserverlib.RawJSON(roomData),
						},
					)
					req.Response.Rooms.Join[roomID] = joinData
				}
			}
		}
	}

	return pos
}
