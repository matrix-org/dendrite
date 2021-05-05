package storage

import "github.com/matrix-org/dendrite/internal"

type Database interface {
	internal.PartitionStorer
}
