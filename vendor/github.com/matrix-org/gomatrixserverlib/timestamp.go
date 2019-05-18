package gomatrixserverlib

import (
	"time"
)

// A Timestamp is a millisecond posix timestamp.
type Timestamp uint64

// AsTimestamp turns a time.Time into a millisecond posix timestamp.
func AsTimestamp(t time.Time) Timestamp {
	return Timestamp(t.UnixNano() / 1000000)
}

// Time turns a millisecond posix timestamp into a UTC time.Time
func (t Timestamp) Time() time.Time {
	return time.Unix(int64(t)/1000, (int64(t)%1000)*1000000).UTC()
}
