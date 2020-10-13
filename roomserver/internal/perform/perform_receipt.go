package perform

import (
	"context"

	"github.com/matrix-org/dendrite/roomserver/api"
	"github.com/matrix-org/dendrite/roomserver/storage"
)

type Receipter struct {
	DB storage.Database
}

func (r *Receipter) PerformUserReceiptUpdate(ctx context.Context, req *api.PerformUserReceiptUpdateRequest, res *api.PerformUserReceiptUpdateResponse) error {
	return r.DB.StoreReceipt(ctx, req.RoomID, req.ReceiptType, req.UserID, req.EventID)
}

func (r *Receipter) QueryRoomReceipts(
	ctx context.Context,
	req *api.QueryRoomReceiptRequest,
	res *api.QueryRoomReceiptResponse,
) error {
	receipts, err := r.DB.GetRoomReceipts(ctx, req.RoomID, req.TS)
	if err != nil {
		return err
	}
	res.Receipts = receipts
	return nil
}
