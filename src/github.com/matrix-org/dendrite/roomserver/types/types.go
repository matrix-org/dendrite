// Package types provides the types that are used internally within the roomserver.
package types

// A PartitionOffset is the offset into a partition of the input log.
type PartitionOffset struct {
	// The ID of the partition.
	Partition int32
	// The offset into the partition.
	Offset int64
}

// An InvalidInput is a problem that was encountered when processing input.
type InvalidInput struct {
	// The topic the input was read from.
	Topic string
	// The partition the input was read from.
	Partition int32
	// The offset in the partition the input was read from.
	Offset int64
	// The value that errored.
	Value []byte
	// The error itself
	Error string
}
