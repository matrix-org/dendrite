package pushrules

func defaultOverrideRules(userID string) []*Rule {
	return []*Rule{
		&mRuleMasterDefinition,
		&mRuleSuppressNoticesDefinition,
		mRuleInviteForMeDefinition(userID),
		&mRuleMemberEventDefinition,
		&mRuleContainsDisplayNameDefinition,
		&mRuleTombstoneDefinition,
		&mRuleRoomNotifDefinition,
	}
}

const (
	MRuleMaster              = ".m.rule.master"
	MRuleSuppressNotices     = ".m.rule.suppress_notices"
	MRuleInviteForMe         = ".m.rule.invite_for_me"
	MRuleMemberEvent         = ".m.rule.member_event"
	MRuleContainsDisplayName = ".m.rule.contains_display_name"
	MRuleTombstone           = ".m.rule.tombstone"
	MRuleRoomNotif           = ".m.rule.roomnotif"
)

var (
	mRuleMasterDefinition = Rule{
		RuleID:     MRuleMaster,
		Default:    true,
		Enabled:    false,
		Conditions: []*Condition{},
		Actions:    []*Action{{Kind: DontNotifyAction}},
	}
	mRuleSuppressNoticesDefinition = Rule{
		RuleID:  MRuleSuppressNotices,
		Default: true,
		Enabled: true,
		Conditions: []*Condition{
			{
				Kind:    EventMatchCondition,
				Key:     "content.msgtype",
				Pattern: "m.notice",
			},
		},
		Actions: []*Action{{Kind: DontNotifyAction}},
	}
	mRuleMemberEventDefinition = Rule{
		RuleID:  MRuleMemberEvent,
		Default: true,
		Enabled: true,
		Conditions: []*Condition{
			{
				Kind:    EventMatchCondition,
				Key:     "type",
				Pattern: "m.room.member",
			},
		},
		Actions: []*Action{{Kind: DontNotifyAction}},
	}
	mRuleContainsDisplayNameDefinition = Rule{
		RuleID:     MRuleContainsDisplayName,
		Default:    true,
		Enabled:    true,
		Conditions: []*Condition{{Kind: ContainsDisplayNameCondition}},
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
			},
		},
	}
	mRuleTombstoneDefinition = Rule{
		RuleID:  MRuleTombstone,
		Default: true,
		Enabled: true,
		Conditions: []*Condition{
			{
				Kind:    EventMatchCondition,
				Key:     "type",
				Pattern: "m.room.tombstone",
			},
			{
				Kind:    EventMatchCondition,
				Key:     "state_key",
				Pattern: "",
			},
		},
		Actions: []*Action{
			{Kind: NotifyAction},
			{
				Kind:  SetTweakAction,
				Tweak: HighlightTweak,
			},
		},
	}
	mRuleRoomNotifDefinition = Rule{
		RuleID:  MRuleRoomNotif,
		Default: true,
		Enabled: true,
		Conditions: []*Condition{
			{
				Kind:    EventMatchCondition,
				Key:     "content.body",
				Pattern: "@room",
			},
			{
				Kind: SenderNotificationPermissionCondition,
				Key:  "room",
			},
		},
		Actions: []*Action{
			{Kind: NotifyAction},
			{
				Kind:  SetTweakAction,
				Tweak: HighlightTweak,
			},
		},
	}
)

func mRuleInviteForMeDefinition(userID string) *Rule {
	return &Rule{
		RuleID:  MRuleInviteForMe,
		Default: true,
		Enabled: true,
		Conditions: []*Condition{
			{
				Kind:    EventMatchCondition,
				Key:     "type",
				Pattern: "m.room.member",
			},
			{
				Kind:    EventMatchCondition,
				Key:     "content.membership",
				Pattern: "invite",
			},
			{
				Kind:    EventMatchCondition,
				Key:     "state_key",
				Pattern: userID,
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
			},
		},
	}
}
