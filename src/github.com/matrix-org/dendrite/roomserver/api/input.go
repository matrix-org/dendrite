// Package api provides the types that are used to communicate with the roomserver.
package api

const (
	// KindOutlier events fall outside the contiguous event graph.
	// We do not have the state for these events.
	// These events are state events used to authenticate other events.
	// They can become part of the contiguous event graph via backfill.
	KindOutlier = 1
	// KindJoin events start a new contiguous event graph. The first event
	// in the list must be a m.room.memeber event joining this server to
	// the room. This must come with the state at the event.
	KindJoin = 2
	// KindNew events extend the contiguous graph going forwards.
	// They usually don't need state, but may include state if the
	// there was a new event that references an event that we don't
	// have a copy of.
	KindNew = 3
	// KindBackfill events extend the contiguous graph going backwards.
	// They always have state.
	KindBackfill = 4
)

// InputRoomEvent is a matrix room event to add to the room server database.
type InputRoomEvent struct {
	// Whether these events are new, backfilled or outliers.
	Kind int
	// The event JSON for each event to add.
	Event []byte
	// Optional list of state events forming the state before this event.
	// These state events must have already been persisted.
	State []string
}
