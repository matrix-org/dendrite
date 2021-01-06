package shared

import (
	"context"
	"encoding/json"

	eduAPI "github.com/matrix-org/dendrite/eduserver/api"
	"github.com/matrix-org/dendrite/syncapi/types"
	"github.com/matrix-org/gomatrixserverlib"
)

type ReceiptStreamProvider struct {
	StreamProvider
}

func (p *ReceiptStreamProvider) Setup() {
	p.StreamProvider.Setup()

	latest, err := p.DB.Receipts.SelectMaxReceiptID(context.Background(), nil)
	if err != nil {
		return
	}

	p.latest = types.StreamPosition(latest)
}

func (p *ReceiptStreamProvider) Range(
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

	lastPos, receipts, err := p.DB.Receipts.SelectRoomReceiptsAfter(ctx, joinedRooms, from)
	if err != nil {
		return to //fmt.Errorf("unable to select receipts for rooms: %w", err)
	}

	if len(receipts) == 0 || lastPos == 0 {
		return to
	}

	// Group receipts by room, so we can create one ClientEvent for every room
	receiptsByRoom := make(map[string][]eduAPI.OutputReceiptEvent)
	for _, receipt := range receipts {
		receiptsByRoom[receipt.RoomID] = append(receiptsByRoom[receipt.RoomID], receipt)
	}

	for roomID, receipts := range receiptsByRoom {
		jr := req.Response.Rooms.Join[roomID]
		var ok bool

		ev := gomatrixserverlib.ClientEvent{
			Type:   gomatrixserverlib.MReceipt,
			RoomID: roomID,
		}
		content := make(map[string]eduAPI.ReceiptMRead)
		for _, receipt := range receipts {
			var read eduAPI.ReceiptMRead
			if read, ok = content[receipt.EventID]; !ok {
				read = eduAPI.ReceiptMRead{
					User: make(map[string]eduAPI.ReceiptTS),
				}
			}
			read.User[receipt.UserID] = eduAPI.ReceiptTS{TS: receipt.Timestamp}
			content[receipt.EventID] = read
		}
		ev.Content, err = json.Marshal(content)
		if err != nil {
			return to // err
		}

		jr.Ephemeral.Events = append(jr.Ephemeral.Events, ev)
		req.Response.Rooms.Join[roomID] = jr
	}

	return lastPos
}
