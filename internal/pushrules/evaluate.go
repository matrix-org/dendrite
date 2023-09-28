package pushrules

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/matrix-org/gomatrixserverlib"
	"github.com/matrix-org/gomatrixserverlib/spec"
)

// A RuleSetEvaluator encapsulates context to evaluate an event
// against a rule set.
type RuleSetEvaluator struct {
	ec      EvaluationContext
	ruleSet []kindAndRules
}

// An EvaluationContext gives a RuleSetEvaluator access to the
// environment, for rules that require that.
type EvaluationContext interface {
	// UserDisplayName returns the current user's display name.
	UserDisplayName() string

	// RoomMemberCount returns the number of members in the room of
	// the current event.
	RoomMemberCount() (int, error)

	// HasPowerLevel returns whether the user has at least the given
	// power in the room of the current event.
	HasPowerLevel(senderID spec.SenderID, levelKey string) (bool, error)
}

// A kindAndRules is just here to simplify iteration of the (ordered)
// kinds of rules.
type kindAndRules struct {
	Kind  Kind
	Rules []*Rule
}

// NewRuleSetEvaluator creates a new evaluator for the given rule set.
func NewRuleSetEvaluator(ec EvaluationContext, ruleSet *RuleSet) *RuleSetEvaluator {
	return &RuleSetEvaluator{
		ec: ec,
		ruleSet: []kindAndRules{
			{OverrideKind, ruleSet.Override},
			{ContentKind, ruleSet.Content},
			{RoomKind, ruleSet.Room},
			{SenderKind, ruleSet.Sender},
			{UnderrideKind, ruleSet.Underride},
		},
	}
}

// MatchEvent returns the first matching rule. Returns nil if there
// was no match rule.
func (rse *RuleSetEvaluator) MatchEvent(event gomatrixserverlib.PDU, userIDForSender spec.UserIDForSender) (*Rule, error) {
	// TODO: server-default rules have lower priority than user rules,
	// but they are stored together with the user rules. It's a bit
	// unclear what the specification (11.14.1.4 Predefined rules)
	// means the ordering should be.
	//
	// The most reasonable interpretation is that default overrides
	// still have lower priority than user content rules, so we
	// iterate twice.
	for _, rsat := range rse.ruleSet {
		for _, defRules := range []bool{false, true} {
			for _, rule := range rsat.Rules {
				if rule.Default != defRules {
					continue
				}
				ok, err := ruleMatches(rule, rsat.Kind, event, rse.ec, userIDForSender)
				if err != nil {
					return nil, err
				}
				if ok {
					return rule, nil
				}
			}
		}
	}

	// No matching rule.
	return nil, nil
}

func ruleMatches(rule *Rule, kind Kind, event gomatrixserverlib.PDU, ec EvaluationContext, userIDForSender spec.UserIDForSender) (bool, error) {
	if !rule.Enabled {
		return false, nil
	}

	switch kind {
	case OverrideKind, UnderrideKind:
		for _, cond := range rule.Conditions {
			ok, err := conditionMatches(cond, event, ec)
			if err != nil {
				return false, err
			}
			if !ok {
				return false, nil
			}
		}
		return true, nil

	case ContentKind:
		// TODO: "These configure behaviour for (unencrypted) messages
		// that match certain patterns." - Does that mean "content.body"?
		if rule.Pattern == nil {
			return false, nil
		}
		return patternMatches("content.body", *rule.Pattern, event)

	case RoomKind:
		return rule.RuleID == event.RoomID().String(), nil

	case SenderKind:
		userID := ""
		sender, err := userIDForSender(event.RoomID(), event.SenderID())
		if err == nil {
			userID = sender.String()
		}
		return rule.RuleID == userID, nil

	default:
		return false, nil
	}
}

func conditionMatches(cond *Condition, event gomatrixserverlib.PDU, ec EvaluationContext) (bool, error) {
	switch cond.Kind {
	case EventMatchCondition:
		if cond.Pattern == nil {
			return false, fmt.Errorf("missing condition pattern")
		}
		return patternMatches(cond.Key, *cond.Pattern, event)

	case ContainsDisplayNameCondition:
		return patternMatches("content.body", ec.UserDisplayName(), event)

	case RoomMemberCountCondition:
		cmp, err := parseRoomMemberCountCondition(cond.Is)
		if err != nil {
			return false, fmt.Errorf("parsing room_member_count condition: %w", err)
		}
		n, err := ec.RoomMemberCount()
		if err != nil {
			return false, fmt.Errorf("RoomMemberCount failed: %w", err)
		}
		return cmp(n), nil

	case SenderNotificationPermissionCondition:
		return ec.HasPowerLevel(event.SenderID(), cond.Key)

	default:
		return false, nil
	}
}

func patternMatches(key, pattern string, event gomatrixserverlib.PDU) (bool, error) {
	// It doesn't make sense for an empty pattern to match anything.
	if pattern == "" {
		return false, nil
	}

	re, err := globToRegexp(pattern)
	if err != nil {
		return false, err
	}

	var eventMap map[string]interface{}
	if err = json.Unmarshal(event.JSON(), &eventMap); err != nil {
		return false, fmt.Errorf("parsing event: %w", err)
	}
	// From the spec:
	// "If the property specified by key is completely absent from
	// the event, or does not have a string value, then the condition
	// will not match, even if pattern is *."
	v, err := lookupMapPath(strings.Split(key, "."), eventMap)
	if err != nil {
		// An unknown path is a benign error that shouldn't stop rule
		// processing. It's just a non-match.
		return false, nil
	}
	if _, ok := v.(string); !ok {
		// A non-string never matches.
		return false, nil
	}

	return re.MatchString(fmt.Sprint(v)), nil
}
