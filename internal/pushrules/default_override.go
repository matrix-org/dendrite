package pushrules

func defaultOverrideRules(userID string) []*Rule {
	return []*Rule{
		&mRuleMasterDefinition,
		&mRuleSuppressNoticesDefinition,
		mRuleInviteForMeDefinition(userID),
		&mRuleMemberEventDefinition,
		&mRuleContainsDisplayNameDefinition,
		&mRuleRoomNotifDefinition,
		&mRuleTombstoneDefinition,
		&mRuleReactionDefinition,
		&mRuleACLsDefinition,
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
	MRuleReaction            = ".m.rule.reaction"
	MRuleRoomACLs            = ".m.rule.room.server_acl"
)

var (
	mRuleMasterDefinition = Rule{
		RuleID:  MRuleMaster,
		Default: true,
		Enabled: false,
		Actions: []*Action{},
	}
	mRuleSuppressNoticesDefinition = Rule{
		RuleID:  MRuleSuppressNotices,
		Default: true,
		Enabled: true,
		Conditions: []*Condition{
			{
				Kind:    EventMatchCondition,
				Key:     "content.msgtype",
				Pattern: pointer("m.notice"),
			},
		},
		Actions: []*Action{},
	}
	mRuleMemberEventDefinition = Rule{
		RuleID:  MRuleMemberEvent,
		Default: true,
		Enabled: true,
		Conditions: []*Condition{
			{
				Kind:    EventMatchCondition,
				Key:     "type",
				Pattern: pointer("m.room.member"),
			},
		},
		Actions: []*Action{},
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
				Pattern: pointer("m.room.tombstone"),
			},
			{
				Kind:    EventMatchCondition,
				Key:     "state_key",
				Pattern: pointer(""),
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
	mRuleACLsDefinition = Rule{
		RuleID:  MRuleRoomACLs,
		Default: true,
		Enabled: true,
		Conditions: []*Condition{
			{
				Kind:    EventMatchCondition,
				Key:     "type",
				Pattern: pointer("m.room.server_acl"),
			},
			{
				Kind:    EventMatchCondition,
				Key:     "state_key",
				Pattern: pointer(""),
			},
		},
		Actions: []*Action{},
	}
	mRuleRoomNotifDefinition = Rule{
		RuleID:  MRuleRoomNotif,
		Default: true,
		Enabled: true,
		Conditions: []*Condition{
			{
				Kind:    EventMatchCondition,
				Key:     "content.body",
				Pattern: pointer("@room"),
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
	mRuleReactionDefinition = Rule{
		RuleID:  MRuleReaction,
		Default: true,
		Enabled: true,
		Conditions: []*Condition{
			{
				Kind:    EventMatchCondition,
				Key:     "type",
				Pattern: pointer("m.reaction"),
			},
		},
		Actions: []*Action{},
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
				Pattern: pointer("m.room.member"),
			},
			{
				Kind:    EventMatchCondition,
				Key:     "content.membership",
				Pattern: pointer("invite"),
			},
			{
				Kind:    EventMatchCondition,
				Key:     "state_key",
				Pattern: pointer(userID),
			},
		},
		Actions: []*Action{
			{Kind: NotifyAction},
			{
				Kind:  SetTweakAction,
				Tweak: SoundTweak,
				Value: "default",
			},
		},
	}
}
