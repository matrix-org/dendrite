package pushrules

// An AccountRuleSets carries the rule sets associated with an
// account.
type AccountRuleSets struct {
	Global RuleSet `json:"global"` // Required
}

// A RuleSet contains all the various push rules for an
// account. Listed in decreasing order of priority.
type RuleSet struct {
	Override  []*Rule `json:"override,omitempty"`
	Content   []*Rule `json:"content,omitempty"`
	Room      []*Rule `json:"room,omitempty"`
	Sender    []*Rule `json:"sender,omitempty"`
	Underride []*Rule `json:"underride,omitempty"`
}

// A Rule contains matchers, conditions and final actions. While
// evaluating, at most one rule is considered matching.
//
// Kind and scope are part of the push rules request/responses, but
// not of the core data model.
type Rule struct {
	// RuleID is either a free identifier, or the sender's MXID for
	// SenderKind. Required.
	RuleID string `json:"rule_id"`

	// Default indicates whether this is a server-defined default, or
	// a user-provided rule. Required.
	//
	// The server-default rules have the lowest priority.
	Default bool `json:"default"`

	// Enabled allows the user to disable rules while keeping them
	// around. Required.
	Enabled bool `json:"enabled"`

	// Actions describe the desired outcome, should the rule
	// match. Required.
	Actions []*Action `json:"actions"`

	// Conditions provide the rule's conditions for OverrideKind and
	// UnderrideKind. Not allowed for other kinds.
	Conditions []*Condition `json:"conditions"`

	// Pattern is the body pattern to match for ContentKind. Required
	// for that kind. The interpretation is the same as that of
	// Condition.Pattern.
	Pattern string `json:"pattern"`
}

// Scope only has one valid value. See also AccountRuleSets.
type Scope string

const (
	UnknownScope Scope = ""
	GlobalScope  Scope = "global"
)

// Kind is the type of push rule. See also RuleSet.
type Kind string

const (
	UnknownKind   Kind = ""
	OverrideKind  Kind = "override"
	ContentKind   Kind = "content"
	RoomKind      Kind = "room"
	SenderKind    Kind = "sender"
	UnderrideKind Kind = "underride"
)
