package streams

import (
	"context"

	"github.com/matrix-org/dendrite/syncapi/types"
	userapi "github.com/matrix-org/dendrite/userapi/api"
	"github.com/matrix-org/gomatrixserverlib"
)

type AccountDataStreamProvider struct {
	StreamProvider
	userAPI userapi.UserInternalAPI
}

func (p *AccountDataStreamProvider) Setup() {
	p.StreamProvider.Setup()

	p.latestMutex.Lock()
	defer p.latestMutex.Unlock()

	id, err := p.DB.MaxStreamPositionForAccountData(context.Background())
	if err != nil {
		panic(err)
	}
	p.latest = id
}

func (p *AccountDataStreamProvider) CompleteSync(
	ctx context.Context,
	req *types.SyncRequest,
) types.StreamPosition {
	dataReq := &userapi.QueryAccountDataRequest{
		UserID: req.Device.UserID,
	}
	dataRes := &userapi.QueryAccountDataResponse{}
	if err := p.userAPI.QueryAccountData(ctx, dataReq, dataRes); err != nil {
		req.Log.WithError(err).Error("p.userAPI.QueryAccountData failed")
		return p.LatestPosition(ctx)
	}
	for datatype, databody := range dataRes.GlobalAccountData {
		req.Response.AccountData.Events = append(
			req.Response.AccountData.Events,
			gomatrixserverlib.ClientEvent{
				Type:    datatype,
				Content: gomatrixserverlib.RawJSON(databody),
			},
		)
	}
	for r, j := range req.Response.Rooms.Join {
		for datatype, databody := range dataRes.RoomAccountData[r] {
			j.AccountData.Events = append(
				j.AccountData.Events,
				gomatrixserverlib.ClientEvent{
					Type:    datatype,
					Content: gomatrixserverlib.RawJSON(databody),
				},
			)
			req.Response.Rooms.Join[r] = j
		}
	}

	return p.LatestPosition(ctx)
}

func (p *AccountDataStreamProvider) IncrementalSync(
	ctx context.Context,
	req *types.SyncRequest,
	from, to types.StreamPosition,
) types.StreamPosition {
	r := types.Range{
		From: from,
		To:   to,
	}
	accountDataFilter := gomatrixserverlib.DefaultEventFilter() // TODO: use filter provided in req instead

	dataTypes, err := p.DB.GetAccountDataInRange(
		ctx, req.Device.UserID, r, &accountDataFilter,
	)
	if err != nil {
		req.Log.WithError(err).Error("p.DB.GetAccountDataInRange failed")
		return from
	}

	// Iterate over the rooms
	for roomID, dataTypes := range dataTypes {
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

	return to
}
