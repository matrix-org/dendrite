package sync

import "github.com/matrix-org/dendrite/syncapi/types"

type SyncProvider interface {
	WaitFor()
}

type SyncStream interface {
	GetLatestPosition() types.StreamPosition
}
