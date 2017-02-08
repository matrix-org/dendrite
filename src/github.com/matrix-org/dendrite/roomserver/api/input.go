// Package api provides the types that are used to communicate with the roomserver.
package api

const (
	// KindOutlier event fall outside the contiguous event graph.
	// We do not have the state for these events.
	// These events are state events used to authenticate other events.
	// They can become part of the contiguous event graph via backfill.
	KindOutlier = 1
	// KindJoin event start a new contiguous event graph. The event must be a
	// m.room.member event joining this server to the room. This must come with
	// the state at the event. If the event is contiguous with the existing
	// graph for the room then it is treated as a normal new event.
	KindJoin = 2
	// KindNew event extend the contiguous graph going forwards.
	// They usually don't need state, but may include state if the
	// there was a new event that references an event that we don't
	// have a copy of.
	KindNew = 3
	// KindBackfill event extend the contiguous graph going backwards.
	// They always have state.
	KindBackfill = 4
)

// InputRoomEvent is a matrix room event to add to the room server database.
// TODO: Implement UnmarshalJSON/MarshalJSON in a way that does something sensible with the event JSON.
type InputRoomEvent struct {
	// Whether this event is new, backfilled or an outlier.
	// This controls how the event is processed.
	Kind int
	// The event JSON for the event to add.
	Event []byte
	// List of state event IDs that authenticate this event.
	AuthEventIDs []string
	// Optional list of state event IDs forming the state before this event.
	// These state events must have already been persisted.
	StateEventIDs []string
}
