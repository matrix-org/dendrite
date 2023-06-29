package pushrules

import (
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/matrix-org/gomatrixserverlib"
	"github.com/matrix-org/gomatrixserverlib/spec"
)

func UserIDForSender(roomID spec.RoomID, senderID spec.SenderID) (*spec.UserID, error) {
	return spec.NewUserID(string(senderID), true)
}

func TestRuleSetEvaluatorMatchEvent(t *testing.T) {
	ev := mustEventFromJSON(t, `{}`)
	defaultEnabled := &Rule{
		RuleID:  ".default.enabled",
		Default: true,
		Enabled: true,
	}
	userEnabled := &Rule{
		RuleID:  ".user.enabled",
		Default: false,
		Enabled: true,
	}
	userEnabled2 := &Rule{
		RuleID:  ".user.enabled.2",
		Default: false,
		Enabled: true,
	}
	defaultRuleset := DefaultGlobalRuleSet("test", "test")
	tsts := []struct {
		Name    string
		RuleSet RuleSet
		Want    *Rule
		Event   gomatrixserverlib.PDU
	}{
		{"empty", RuleSet{}, nil, ev},
		{"defaultCanWin", RuleSet{Override: []*Rule{defaultEnabled}}, defaultEnabled, ev},
		{"userWins", RuleSet{Override: []*Rule{defaultEnabled, userEnabled}}, userEnabled, ev},
		{"defaultOverrideWins", RuleSet{Override: []*Rule{defaultEnabled}, Underride: []*Rule{userEnabled}}, defaultEnabled, ev},
		{"overrideContent", RuleSet{Override: []*Rule{userEnabled}, Content: []*Rule{userEnabled2}}, userEnabled, ev},
		{"overrideRoom", RuleSet{Override: []*Rule{userEnabled}, Room: []*Rule{userEnabled2}}, userEnabled, ev},
		{"overrideSender", RuleSet{Override: []*Rule{userEnabled}, Sender: []*Rule{userEnabled2}}, userEnabled, ev},
		{"overrideUnderride", RuleSet{Override: []*Rule{userEnabled}, Underride: []*Rule{userEnabled2}}, userEnabled, ev},
		{"reactions don't notify", *defaultRuleset, &mRuleReactionDefinition, mustEventFromJSON(t, `{"type":"m.reaction"}`)},
		{"receipts don't notify", *defaultRuleset, nil, mustEventFromJSON(t, `{"type":"m.receipt"}`)},
	}
	for _, tst := range tsts {
		t.Run(tst.Name, func(t *testing.T) {
			rse := NewRuleSetEvaluator(fakeEvaluationContext{3}, &tst.RuleSet)
			got, err := rse.MatchEvent(tst.Event, UserIDForSender)
			if err != nil {
				t.Fatalf("MatchEvent failed: %v", err)
			}
			if diff := cmp.Diff(tst.Want, got); diff != "" {
				t.Errorf("MatchEvent rule: +got -want:\n%s", diff)
			}
		})
	}
}

func TestRuleMatches(t *testing.T) {
	emptyRule := Rule{Enabled: true}
	tsts := []struct {
		Name      string
		Kind      Kind
		Rule      Rule
		EventJSON string
		Want      bool
	}{
		{"emptyOverride", OverrideKind, emptyRule, `{}`, true},
		{"emptyContent", ContentKind, emptyRule, `{}`, false},
		{"emptyRoom", RoomKind, emptyRule, `{}`, true},
		{"emptySender", SenderKind, emptyRule, `{"room_id":"!room:example.com"}`, true},
		{"emptyUnderride", UnderrideKind, emptyRule, `{}`, true},

		{"disabled", OverrideKind, Rule{}, `{}`, false},

		{"overrideConditionMatch", OverrideKind, Rule{Enabled: true}, `{}`, true},
		{"overrideConditionNoMatch", OverrideKind, Rule{Enabled: true, Conditions: []*Condition{{}}}, `{}`, false},

		{"underrideConditionMatch", UnderrideKind, Rule{Enabled: true}, `{}`, true},
		{"underrideConditionNoMatch", UnderrideKind, Rule{Enabled: true, Conditions: []*Condition{{}}}, `{}`, false},

		{"contentMatch", ContentKind, Rule{Enabled: true, Pattern: pointer("b")}, `{"content":{"body":"abc"}}`, true},
		{"contentNoMatch", ContentKind, Rule{Enabled: true, Pattern: pointer("d")}, `{"content":{"body":"abc"}}`, false},

		{"roomMatch", RoomKind, Rule{Enabled: true, RuleID: "!room:example.com"}, `{"room_id":"!room:example.com"}`, true},
		{"roomNoMatch", RoomKind, Rule{Enabled: true, RuleID: "!room:example.com"}, `{"room_id":"!otherroom:example.com"}`, false},

		{"senderMatch", SenderKind, Rule{Enabled: true, RuleID: "@user:example.com"}, `{"sender":"@user:example.com","room_id":"!room:example.com"}`, true},
		{"senderNoMatch", SenderKind, Rule{Enabled: true, RuleID: "@user:example.com"}, `{"sender":"@otheruser:example.com","room_id":"!room:example.com"}`, false},
	}
	for _, tst := range tsts {
		t.Run(tst.Name, func(t *testing.T) {
			got, err := ruleMatches(&tst.Rule, tst.Kind, mustEventFromJSON(t, tst.EventJSON), nil, UserIDForSender)
			if err != nil {
				t.Fatalf("ruleMatches failed: %v", err)
			}
			if got != tst.Want {
				t.Errorf("ruleMatches: got %v, want %v", got, tst.Want)
			}
		})
	}
}

func TestConditionMatches(t *testing.T) {
	tsts := []struct {
		Name      string
		Cond      Condition
		EventJSON string
		WantMatch bool
		WantErr   bool
	}{
		{Name: "empty", Cond: Condition{}, EventJSON: `{}`, WantMatch: false, WantErr: false},
		{Name: "empty", Cond: Condition{Kind: "unknownstring"}, EventJSON: `{}`, WantMatch: false, WantErr: false},

		// Neither of these should match because `content` is not a full string match,
		// and `content.body` is not a string value.
		{Name: "eventMatch", Cond: Condition{Kind: EventMatchCondition, Key: "content", Pattern: pointer("")}, EventJSON: `{"content":{}}`, WantMatch: false, WantErr: false},
		{Name: "eventBodyMatch", Cond: Condition{Kind: EventMatchCondition, Key: "content.body", Is: "3", Pattern: pointer("")}, EventJSON: `{"content":{"body": "3"}}`, WantMatch: false, WantErr: false},
		{Name: "eventBodyMatch matches", Cond: Condition{Kind: EventMatchCondition, Key: "content.body", Pattern: pointer("world")}, EventJSON: `{"content":{"body": "hello world!"}}`, WantMatch: true, WantErr: false},
		{Name: "EventMatch missing pattern", Cond: Condition{Kind: EventMatchCondition, Key: "content.body"}, EventJSON: `{"content":{"body": "hello world!"}}`, WantMatch: false, WantErr: true},

		{Name: "displayNameNoMatch", Cond: Condition{Kind: ContainsDisplayNameCondition}, EventJSON: `{"content":{"body":"something without displayname"}}`, WantMatch: false, WantErr: false},
		{Name: "displayNameMatch", Cond: Condition{Kind: ContainsDisplayNameCondition}, EventJSON: `{"content":{"body":"hello Dear User, how are you?"}}`, WantMatch: true, WantErr: false},

		{Name: "roomMemberCountLessNoMatch", Cond: Condition{Kind: RoomMemberCountCondition, Is: "<2"}, EventJSON: `{}`, WantMatch: false, WantErr: false},
		{Name: "roomMemberCountLessMatch", Cond: Condition{Kind: RoomMemberCountCondition, Is: "<3"}, EventJSON: `{}`, WantMatch: true, WantErr: false},
		{Name: "roomMemberCountLessEqualNoMatch", Cond: Condition{Kind: RoomMemberCountCondition, Is: "<=1"}, EventJSON: `{}`, WantMatch: false, WantErr: false},
		{Name: "roomMemberCountLessEqualMatch", Cond: Condition{Kind: RoomMemberCountCondition, Is: "<=2"}, EventJSON: `{}`, WantMatch: true, WantErr: false},
		{Name: "roomMemberCountEqualNoMatch", Cond: Condition{Kind: RoomMemberCountCondition, Is: "==1"}, EventJSON: `{}`, WantMatch: false, WantErr: false},
		{Name: "roomMemberCountEqualMatch", Cond: Condition{Kind: RoomMemberCountCondition, Is: "==2"}, EventJSON: `{}`, WantMatch: true, WantErr: false},
		{Name: "roomMemberCountGreaterEqualNoMatch", Cond: Condition{Kind: RoomMemberCountCondition, Is: ">=3"}, EventJSON: `{}`, WantMatch: false, WantErr: false},
		{Name: "roomMemberCountGreaterEqualMatch", Cond: Condition{Kind: RoomMemberCountCondition, Is: ">=2"}, EventJSON: `{}`, WantMatch: true, WantErr: false},
		{Name: "roomMemberCountGreaterNoMatch", Cond: Condition{Kind: RoomMemberCountCondition, Is: ">2"}, EventJSON: `{}`, WantMatch: false, WantErr: false},
		{Name: "roomMemberCountGreaterMatch", Cond: Condition{Kind: RoomMemberCountCondition, Is: ">1"}, EventJSON: `{}`, WantMatch: true, WantErr: false},

		{Name: "senderNotificationPermissionMatch", Cond: Condition{Kind: SenderNotificationPermissionCondition, Key: "powerlevel"}, EventJSON: `{"sender":"@poweruser:example.com"}`, WantMatch: true, WantErr: false},
		{Name: "senderNotificationPermissionNoMatch", Cond: Condition{Kind: SenderNotificationPermissionCondition, Key: "powerlevel"}, EventJSON: `{"sender":"@nobody:example.com"}`, WantMatch: false, WantErr: false},
	}
	for _, tst := range tsts {
		t.Run(tst.Name, func(t *testing.T) {
			got, err := conditionMatches(&tst.Cond, mustEventFromJSON(t, tst.EventJSON), &fakeEvaluationContext{2})
			if err != nil && !tst.WantErr {
				t.Fatalf("conditionMatches failed: %v", err)
			}
			if got != tst.WantMatch {
				t.Errorf("conditionMatches: got %v, want %v on %s", got, tst.WantMatch, tst.Name)
			}
		})
	}
}

type fakeEvaluationContext struct{ memberCount int }

func (fakeEvaluationContext) UserDisplayName() string         { return "Dear User" }
func (f fakeEvaluationContext) RoomMemberCount() (int, error) { return f.memberCount, nil }
func (fakeEvaluationContext) HasPowerLevel(senderID spec.SenderID, levelKey string) (bool, error) {
	return senderID == "@poweruser:example.com" && levelKey == "powerlevel", nil
}

func TestPatternMatches(t *testing.T) {
	tsts := []struct {
		Name      string
		Key       string
		Pattern   string
		EventJSON string
		Want      bool
	}{
		{"empty", "", "", `{}`, false},

		{"patternEmpty", "content", "", `{"content":{}}`, false},

		{"literal", "content.creator", "acreator", `{"content":{"creator":"acreator"}}`, true},
		{"substring", "content.creator", "reat", `{"content":{"creator":"acreator"}}`, true},
		{"singlePattern", "content.creator", "acr?ator", `{"content":{"creator":"acreator"}}`, true},
		{"multiPattern", "content.creator", "a*ea*r", `{"content":{"creator":"acreator"}}`, true},
		{"patternNoSubstring", "content.creator", "r*t", `{"content":{"creator":"acreator"}}`, false},
	}
	for _, tst := range tsts {
		t.Run(tst.Name, func(t *testing.T) {
			got, err := patternMatches(tst.Key, tst.Pattern, mustEventFromJSON(t, tst.EventJSON))
			if err != nil {
				t.Fatalf("patternMatches failed: %v", err)
			}
			if got != tst.Want {
				t.Errorf("patternMatches: got %v, want %v on %s", got, tst.Want, tst.Name)
			}
		})
	}
}

func mustEventFromJSON(t *testing.T, json string) gomatrixserverlib.PDU {
	ev, err := gomatrixserverlib.MustGetRoomVersion(gomatrixserverlib.RoomVersionV7).NewEventFromTrustedJSON([]byte(json), false)
	if err != nil {
		t.Fatal(err)
	}
	return ev
}
