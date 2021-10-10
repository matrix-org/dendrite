package pushrules

// A Condition dictates extra conditions for a matching rules. See
// ConditionKind.
type Condition struct {
	// Kind is the primary discriminator for the condition
	// type. Required.
	Kind ConditionKind `json:"kind"`

	// Key indicates the dot-separated path of Event fields to
	// match. Required for EventMatchCondition and
	// SenderNotificationPermissionCondition.
	Key string `json:"key,omitempty"`

	// Pattern indicates the value pattern that must match. Required
	// for EventMatchCondition.
	Pattern string `json:"pattern,omitempty"`

	// Is indicates the condition that must be fulfilled. Required for
	// RoomMemberCountCondition.
	Is string `json:"is,omitempty"`
}

// ConditionKind represents a kind of condition.
//
// SPEC: Unrecognised conditions MUST NOT match any events,
// effectively making the push rule disabled.
type ConditionKind string

const (
	UnknownCondition ConditionKind = ""

	// EventMatchCondition indicates the condition looks for a key
	// path and matches a pattern. How paths that don't reference a
	// simple value match against rules is implementation-specific.
	EventMatchCondition ConditionKind = "event_match"

	// ContainsDisplayNameCondition indicates the current user's
	// display name must be found in the content body.
	ContainsDisplayNameCondition ConditionKind = "contains_display_name"

	// RoomMemberCountCondition matches a simple arithmetic comparison
	// against the total number of members in a room.
	RoomMemberCountCondition ConditionKind = "room_member_count"

	// SenderNotificationPermissionCondition compares power level for
	// the sender in the event's room.
	SenderNotificationPermissionCondition ConditionKind = "sender_notification_permission"
)
