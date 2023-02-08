package pushrules

func defaultContentRules(localpart string) []*Rule {
	return []*Rule{
		mRuleContainsUserNameDefinition(localpart),
	}
}

const (
	MRuleContainsUserName = ".m.rule.contains_user_name"
)

func mRuleContainsUserNameDefinition(localpart string) *Rule {
	return &Rule{
		RuleID:  MRuleContainsUserName,
		Default: true,
		Enabled: true,
		Pattern: localpart,
		Conditions: []*Condition{
			{
				Kind: EventMatchCondition,
				Key:  "content.body",
			},
		},
		Actions: []*Action{
			{Kind: NotifyAction},
			{
				Kind:  SetTweakAction,
				Tweak: SoundTweak,
				Value: "default",
			},
			{
				Kind:  SetTweakAction,
				Tweak: HighlightTweak,
				Value: true,
			},
		},
	}
}
