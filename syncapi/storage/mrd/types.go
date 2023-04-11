package mrd

type StateEvent struct {
	Hidden   bool  `json:"hidden"`
	ExpireTs int64 `json:"expire_ts"`
}
