package api

type PerformUserReceiptUpdate struct {
	RoomID      string
	ReceiptType string
	EventID     string
	UserID      string
}

type PerformUserReceiptUpdateResponse struct{}
