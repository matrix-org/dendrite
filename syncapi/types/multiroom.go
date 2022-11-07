package types

type MultiRoom map[string]map[string]MultiRoomData

type MultiRoomContent []byte

type MultiRoomData struct {
	Content   MultiRoomContent `json:"content"`
	Timestamp int64            `json:"timestamp"`
}

func (d MultiRoomContent) MarshalJSON() ([]byte, error) {
	return d, nil
}

type MultiRoomDataRow struct {
	Data      []byte
	Type      string
	UserId    string
	Timestamp int64
}
