package streams

import (
	"context"
	"encoding/json"

	"github.com/matrix-org/gomatrixserverlib"

	"github.com/matrix-org/dendrite/syncapi/storage"
	"github.com/matrix-org/dendrite/syncapi/types"
)

type ReceiptStreamProvider struct {
	DefaultStreamProvider
}

func (p *ReceiptStreamProvider) Setup(
	ctx context.Context, snapshot storage.DatabaseTransaction,
) {
	p.DefaultStreamProvider.Setup(ctx, snapshot)

	p.latestMutex.Lock()
	defer p.latestMutex.Unlock()

	id, err := snapshot.MaxStreamPositionForReceipts(ctx)
	if err != nil {
		panic(err)
	}
	p.latest = id
}

func (p *ReceiptStreamProvider) CompleteSync(
	ctx context.Context,
	snapshot storage.DatabaseTransaction,
	req *types.SyncRequest,
) types.StreamPosition {
	return p.IncrementalSync(ctx, snapshot, req, 0, p.LatestPosition(ctx))
}

func (p *ReceiptStreamProvider) IncrementalSync(
	ctx context.Context,
	snapshot storage.DatabaseTransaction,
	req *types.SyncRequest,
	from, to types.StreamPosition,
) types.StreamPosition {
	var joinedRooms []string
	for roomID, membership := range req.Rooms {
		if membership == gomatrixserverlib.Join {
			joinedRooms = append(joinedRooms, roomID)
		}
	}

	lastPos, receipts, err := snapshot.RoomReceiptsAfter(ctx, joinedRooms, from)
	if err != nil {
		req.Log.WithError(err).Error("p.DB.RoomReceiptsAfter failed")
		return from
	}

	if len(receipts) == 0 || lastPos == 0 {
		return to
	}

	// Group receipts by room, so we can create one ClientEvent for every room
	receiptsByRoom := make(map[string][]types.OutputReceiptEvent)
	for _, receipt := range receipts {
		// skip ignored user events
		if _, ok := req.IgnoredUsers.List[receipt.UserID]; ok {
			continue
		}
		// Don't send private read receipts to other users
		if receipt.Type == "m.read.private" && req.Device.UserID != receipt.UserID {
			continue
		}
		receiptsByRoom[receipt.RoomID] = append(receiptsByRoom[receipt.RoomID], receipt)
	}

	for roomID, receipts := range receiptsByRoom {
		// For a complete sync, make sure we're only including this room if
		// that room was present in the joined rooms.
		if from == 0 && !req.IsRoomPresent(roomID) {
			continue
		}

		jr, ok := req.Response.Rooms.Join[roomID]
		if !ok {
			jr = types.NewJoinResponse()
		}

		ev := gomatrixserverlib.ClientEvent{
			Type: gomatrixserverlib.MReceipt,
		}
		content := make(map[string]ReceiptMRead)
		for _, receipt := range receipts {
			read, ok := content[receipt.EventID]
			if !ok {
				read = ReceiptMRead{
					User: make(map[string]ReceiptTS),
				}
			}
			read.User[receipt.UserID] = ReceiptTS{TS: receipt.Timestamp}
			content[receipt.EventID] = read
		}
		ev.Content, err = json.Marshal(content)
		if err != nil {
			req.Log.WithError(err).Error("json.Marshal failed")
			return from
		}

		jr.Ephemeral.Events = append(jr.Ephemeral.Events, ev)
		req.Response.Rooms.Join[roomID] = jr
	}

	return lastPos
}

type ReceiptMRead struct {
	User map[string]ReceiptTS `json:"m.read"`
}

type ReceiptTS struct {
	TS gomatrixserverlib.Timestamp `json:"ts"`
}
