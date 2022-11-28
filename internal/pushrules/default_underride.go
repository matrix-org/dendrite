package pushrules

const (
	MRuleCall                  = ".m.rule.call"
	MRuleEncryptedRoomOneToOne = ".m.rule.encrypted_room_one_to_one"
	MRuleRoomOneToOne          = ".m.rule.room_one_to_one"
	MRuleMessage               = ".m.rule.message"
	MRuleEncrypted             = ".m.rule.encrypted"
)

var defaultUnderrideRules = []*Rule{
	&mRuleCallDefinition,
	&mRuleRoomOneToOneDefinition,
	&mRuleEncryptedRoomOneToOneDefinition,
	&mRuleMessageDefinition,
	&mRuleEncryptedDefinition,
}

var (
	mRuleCallDefinition = Rule{
		RuleID:  MRuleCall,
		Default: true,
		Enabled: true,
		Conditions: []*Condition{
			{
				Kind:    EventMatchCondition,
				Key:     "type",
				Pattern: "m.call.invite",
			},
		},
		Actions: []*Action{
			{Kind: NotifyAction},
			{
				Kind:  SetTweakAction,
				Tweak: SoundTweak,
				Value: "ring",
			},
		},
	}
	mRuleEncryptedRoomOneToOneDefinition = Rule{
		RuleID:  MRuleEncryptedRoomOneToOne,
		Default: true,
		Enabled: true,
		Conditions: []*Condition{
			{
				Kind: RoomMemberCountCondition,
				Is:   "2",
			},
			{
				Kind:    EventMatchCondition,
				Key:     "type",
				Pattern: "m.room.encrypted",
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
	mRuleRoomOneToOneDefinition = Rule{
		RuleID:  MRuleRoomOneToOne,
		Default: true,
		Enabled: true,
		Conditions: []*Condition{
			{
				Kind: RoomMemberCountCondition,
				Is:   "2",
			},
			{
				Kind:    EventMatchCondition,
				Key:     "type",
				Pattern: "m.room.message",
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
	mRuleMessageDefinition = Rule{
		RuleID:  MRuleMessage,
		Default: true,
		Enabled: true,
		Conditions: []*Condition{
			{
				Kind:    EventMatchCondition,
				Key:     "type",
				Pattern: "m.room.message",
			},
		},
		Actions: []*Action{
			{Kind: NotifyAction},
		},
	}
	mRuleEncryptedDefinition = Rule{
		RuleID:  MRuleEncrypted,
		Default: true,
		Enabled: true,
		Conditions: []*Condition{
			{
				Kind:    EventMatchCondition,
				Key:     "type",
				Pattern: "m.room.encrypted",
			},
		},
		Actions: []*Action{
			{Kind: NotifyAction},
		},
	}
)
