package streams

import (
	"context"
	"encoding/json"

	"github.com/matrix-org/dendrite/syncapi/types"
	"github.com/matrix-org/gomatrixserverlib"
)

type ReceiptStreamProvider struct {
	StreamProvider
}

func (p *ReceiptStreamProvider) Setup() {
	p.StreamProvider.Setup()

	id, err := p.DB.MaxStreamPositionForReceipts(context.Background())
	if err != nil {
		panic(err)
	}
	p.latest = id
}

func (p *ReceiptStreamProvider) CompleteSync(
	ctx context.Context,
	req *types.SyncRequest,
) types.StreamPosition {
	return p.IncrementalSync(ctx, req, 0, p.LatestPosition(ctx))
}

func (p *ReceiptStreamProvider) IncrementalSync(
	ctx context.Context,
	req *types.SyncRequest,
	from, to types.StreamPosition,
) types.StreamPosition {
	var joinedRooms []string
	for roomID, membership := range req.Rooms {
		if membership == gomatrixserverlib.Join {
			joinedRooms = append(joinedRooms, roomID)
		}
	}

	lastPos, receipts, err := p.DB.RoomReceiptsAfter(ctx, joinedRooms, from)
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
		receiptsByRoom[receipt.RoomID] = append(receiptsByRoom[receipt.RoomID], receipt)
	}

	for roomID, receipts := range receiptsByRoom {
		// For a complete sync, make sure we're only including this room if
		// that room was present in the joined rooms.
		if from == 0 && !req.IsRoomPresent(roomID) {
			continue
		}

		jr := *types.NewJoinResponse()
		if existing, ok := req.Response.Rooms.Join[roomID]; ok {
			jr = existing
		}

		ev := gomatrixserverlib.ClientEvent{
			Type:   gomatrixserverlib.MReceipt,
			RoomID: roomID,
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
