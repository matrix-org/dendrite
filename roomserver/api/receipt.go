package api

type PerformUserReceiptUpdateRequest struct {
	RoomID      string `json:"room_id"`
	ReceiptType string `json:"type"`
	EventID     string `json:"event_id"`
	UserID      string `json:"user_id"`
}

type PerformUserReceiptUpdateResponse struct{}

type ReceiptEvent struct {
	Content map[string]struct {
		Data map[string]TS `json:"m.read"`
	} `json:"content"`
	RoomID string `json:"room_id"`
	Type   string `json:"type"`
}

type TS struct {
	TimeStamp int `json:"ts"`
}
