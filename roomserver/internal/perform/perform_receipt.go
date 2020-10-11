package perform

import (
	"context"

	"github.com/matrix-org/dendrite/roomserver/api"
	"github.com/matrix-org/dendrite/roomserver/storage"
)

type Receipter struct {
	DB storage.Database
}

func (r *Receipter) PerformUserReceiptUpdate(ctx context.Context, req *api.PerformUserReceiptUpdate, res *api.PerformUserReceiptUpdateResponse) error {
	return r.DB.StoreReceipt(ctx, req.RoomID, req.ReceiptType, req.UserID, req.EventID)
}
