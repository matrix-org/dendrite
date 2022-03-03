package pushrules

import (
	"github.com/matrix-org/gomatrixserverlib"
)

// DefaultAccountRuleSets is the complete set of default push rules
// for an account.
func DefaultAccountRuleSets(localpart string, serverName gomatrixserverlib.ServerName) *AccountRuleSets {
	return &AccountRuleSets{
		Global: *DefaultGlobalRuleSet(localpart, serverName),
	}
}

// DefaultGlobalRuleSet returns the default ruleset for a given (fully
// qualified) MXID.
func DefaultGlobalRuleSet(localpart string, serverName gomatrixserverlib.ServerName) *RuleSet {
	return &RuleSet{
		Override:  defaultOverrideRules("@" + localpart + ":" + string(serverName)),
		Content:   defaultContentRules(localpart),
		Underride: defaultUnderrideRules,
	}
}
