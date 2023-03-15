// Code generated by sqlc. DO NOT EDIT.
// versions:
//   sqlc v1.15.0

package mrd

import (
	"time"
)

type SyncapiMultiroomDatum struct {
	ID     int64     `json:"id"`
	UserID string    `json:"user_id"`
	Type   string    `json:"type"`
	Data   []byte    `json:"data"`
	Ts     time.Time `json:"ts"`
}

type SyncapiMultiroomVisibility struct {
	UserID   string `json:"user_id"`
	Type     string `json:"type"`
	RoomID   string `json:"room_id"`
	ExpireTs int64  `json:"expire_ts"`
}
