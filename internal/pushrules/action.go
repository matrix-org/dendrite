package pushrules

import (
	"bytes"
	"encoding/json"
	"fmt"
)

// An Action is (part of) an outcome of a rule. There are
// (unofficially) terminal actions, and modifier actions.
type Action struct {
	// Kind is the type of action. Has custom encoding in JSON.
	Kind ActionKind `json:"-"`

	// Tweak is the property to tweak. Has custom encoding in JSON.
	Tweak TweakKey `json:"-"`

	// Value is some value interpreted according to Kind and Tweak.
	Value interface{} `json:"value"`
}

func (a *Action) MarshalJSON() ([]byte, error) {
	if a.Tweak == UnknownTweak && a.Value == nil {
		return json.Marshal(a.Kind)
	}

	if a.Kind != SetTweakAction {
		return nil, fmt.Errorf("only set_tweak actions may have a value, but got kind %q", a.Kind)
	}

	m := map[string]interface{}{
		string(a.Kind): a.Tweak,
	}
	if a.Value != nil {
		m["value"] = a.Value
	}

	return json.Marshal(m)
}

func (a *Action) UnmarshalJSON(bs []byte) error {
	if bytes.HasPrefix(bs, []byte("\"")) {
		return json.Unmarshal(bs, &a.Kind)
	}

	var raw struct {
		SetTweak TweakKey    `json:"set_tweak"`
		Value    interface{} `json:"value"`
	}
	if err := json.Unmarshal(bs, &raw); err != nil {
		return err
	}
	if raw.SetTweak == UnknownTweak {
		return fmt.Errorf("got unknown action JSON: %s", string(bs))
	}
	a.Kind = SetTweakAction
	a.Tweak = raw.SetTweak
	if raw.Value != nil {
		a.Value = raw.Value
	}

	return nil
}

// ActionKind is the primary discriminator for actions.
type ActionKind string

const (
	UnknownAction ActionKind = ""

	// NotifyAction indicates the clients should show a notification.
	NotifyAction ActionKind = "notify"

	// DontNotifyAction indicates the clients should not show a notification.
	DontNotifyAction ActionKind = "dont_notify"

	// CoalesceAction tells the clients to show a notification, and
	// tells both servers and clients that multiple events can be
	// coalesced into a single notification. The behaviour is
	// implementation-specific.
	CoalesceAction ActionKind = "coalesce"

	// SetTweakAction uses the Tweak and Value fields to add a
	// tweak. Multiple SetTweakAction can be provided in a rule,
	// combined with NotifyAction or CoalesceAction.
	SetTweakAction ActionKind = "set_tweak"
)

// A TweakKey describes a property to be modified/tweaked for events
// that match the rule.
type TweakKey string

const (
	UnknownTweak TweakKey = ""

	// SoundTweak describes which sound to play. Using "default" means
	// "enable sound".
	SoundTweak TweakKey = "sound"

	// HighlightTweak asks the clients to highlight the conversation.
	HighlightTweak TweakKey = "highlight"
)
