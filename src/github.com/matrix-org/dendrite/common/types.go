package common

// StateKeyTuple is a pair of an event type and state_key.
// This is typically used as a key in a map.
type StateKeyTuple struct {
	// The "type" key of a matrix event.
	EventType string
	// The "state_key" of a matrix event.
	// The empty string is a legitimate value for the "state_key" in matrix
	// so take care to initialise this field lest you accidentally request a
	// "state_key" with the go default of the empty string.
	EventStateKey string
}
