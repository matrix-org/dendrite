// Package types provides the types that are used internally within the roomserver.
package types

// A PartitionOffset is the offset into a partition of the input log.
type PartitionOffset struct {
	// The ID of the partition.
	Partition int32
	// The offset into the partition.
	Offset int64
}
