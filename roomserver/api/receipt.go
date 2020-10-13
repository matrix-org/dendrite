package api

type PerformUserReceiptUpdateRequest struct {
	RoomID      string
	ReceiptType string
	EventID     string
	UserID      string
}

type PerformUserReceiptUpdateResponse struct{}
