package pushrules

import (
	"fmt"
	"regexp"
)

// ValidateRule checks the rule for errors. These follow from Sytests
// and the specification.
func ValidateRule(kind Kind, rule *Rule) []error {
	var errs []error

	if !validRuleIDRE.MatchString(rule.RuleID) {
		errs = append(errs, fmt.Errorf("invalid rule ID: %s", rule.RuleID))
	}

	if len(rule.Actions) == 0 {
		errs = append(errs, fmt.Errorf("missing actions"))
	}
	for _, action := range rule.Actions {
		errs = append(errs, validateAction(action)...)
	}

	for _, cond := range rule.Conditions {
		errs = append(errs, validateCondition(cond)...)
	}

	switch kind {
	case OverrideKind, UnderrideKind:
		// The empty list is allowed, but for JSON-encoding reasons,
		// it must not be nil.
		if rule.Conditions == nil {
			errs = append(errs, fmt.Errorf("missing rule conditions"))
		}

	case ContentKind:
		if rule.Pattern == "" {
			errs = append(errs, fmt.Errorf("missing content rule pattern"))
		}

	case RoomKind, SenderKind:
		// Do nothing.

	default:
		errs = append(errs, fmt.Errorf("invalid rule kind: %s", kind))
	}

	return errs
}

// validRuleIDRE is a regexp for valid IDs.
//
// TODO: the specification doesn't seem to say what the rule ID syntax
// is. A Sytest fails if it contains a backslash.
var validRuleIDRE = regexp.MustCompile(`^([^\\]+)$`)

// validateAction returns issues with an Action.
func validateAction(action *Action) []error {
	var errs []error

	switch action.Kind {
	case NotifyAction, DontNotifyAction, CoalesceAction, SetTweakAction:
		// Do nothing.

	default:
		errs = append(errs, fmt.Errorf("invalid rule action kind: %s", action.Kind))
	}

	return errs
}

// validateCondition returns issues with a Condition.
func validateCondition(cond *Condition) []error {
	var errs []error

	switch cond.Kind {
	case EventMatchCondition, ContainsDisplayNameCondition, RoomMemberCountCondition, SenderNotificationPermissionCondition:
		// Do nothing.

	default:
		errs = append(errs, fmt.Errorf("invalid rule condition kind: %s", cond.Kind))
	}

	return errs
}
