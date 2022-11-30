package pushrules

import (
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/matrix-org/gomatrixserverlib"
)

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
		Event   *gomatrixserverlib.Event
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
			got, err := rse.MatchEvent(tst.Event)
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
		{"emptySender", SenderKind, emptyRule, `{}`, true},
		{"emptyUnderride", UnderrideKind, emptyRule, `{}`, true},

		{"disabled", OverrideKind, Rule{}, `{}`, false},

		{"overrideConditionMatch", OverrideKind, Rule{Enabled: true}, `{}`, true},
		{"overrideConditionNoMatch", OverrideKind, Rule{Enabled: true, Conditions: []*Condition{{}}}, `{}`, false},

		{"underrideConditionMatch", UnderrideKind, Rule{Enabled: true}, `{}`, true},
		{"underrideConditionNoMatch", UnderrideKind, Rule{Enabled: true, Conditions: []*Condition{{}}}, `{}`, false},

		{"contentMatch", ContentKind, Rule{Enabled: true, Pattern: "b"}, `{"content":{"body":"abc"}}`, true},
		{"contentNoMatch", ContentKind, Rule{Enabled: true, Pattern: "d"}, `{"content":{"body":"abc"}}`, false},

		{"roomMatch", RoomKind, Rule{Enabled: true, RuleID: "!room@example.com"}, `{"room_id":"!room@example.com"}`, true},
		{"roomNoMatch", RoomKind, Rule{Enabled: true, RuleID: "!room@example.com"}, `{"room_id":"!otherroom@example.com"}`, false},

		{"senderMatch", SenderKind, Rule{Enabled: true, RuleID: "@user@example.com"}, `{"sender":"@user@example.com"}`, true},
		{"senderNoMatch", SenderKind, Rule{Enabled: true, RuleID: "@user@example.com"}, `{"sender":"@otheruser@example.com"}`, false},
	}
	for _, tst := range tsts {
		t.Run(tst.Name, func(t *testing.T) {
			got, err := ruleMatches(&tst.Rule, tst.Kind, mustEventFromJSON(t, tst.EventJSON), nil)
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
		Want      bool
	}{
		{"empty", Condition{}, `{}`, false},
		{"empty", Condition{Kind: "unknownstring"}, `{}`, false},

		// Neither of these should match because `content` is not a full string match,
		// and `content.body` is not a string value.
		{"eventMatch", Condition{Kind: EventMatchCondition, Key: "content"}, `{"content":{}}`, false},
		{"eventBodyMatch", Condition{Kind: EventMatchCondition, Key: "content.body", Is: "3"}, `{"content":{"body": 3}}`, false},

		{"displayNameNoMatch", Condition{Kind: ContainsDisplayNameCondition}, `{"content":{"body":"something without displayname"}}`, false},
		{"displayNameMatch", Condition{Kind: ContainsDisplayNameCondition}, `{"content":{"body":"hello Dear User, how are you?"}}`, true},

		{"roomMemberCountLessNoMatch", Condition{Kind: RoomMemberCountCondition, Is: "<2"}, `{}`, false},
		{"roomMemberCountLessMatch", Condition{Kind: RoomMemberCountCondition, Is: "<3"}, `{}`, true},
		{"roomMemberCountLessEqualNoMatch", Condition{Kind: RoomMemberCountCondition, Is: "<=1"}, `{}`, false},
		{"roomMemberCountLessEqualMatch", Condition{Kind: RoomMemberCountCondition, Is: "<=2"}, `{}`, true},
		{"roomMemberCountEqualNoMatch", Condition{Kind: RoomMemberCountCondition, Is: "==1"}, `{}`, false},
		{"roomMemberCountEqualMatch", Condition{Kind: RoomMemberCountCondition, Is: "==2"}, `{}`, true},
		{"roomMemberCountGreaterEqualNoMatch", Condition{Kind: RoomMemberCountCondition, Is: ">=3"}, `{}`, false},
		{"roomMemberCountGreaterEqualMatch", Condition{Kind: RoomMemberCountCondition, Is: ">=2"}, `{}`, true},
		{"roomMemberCountGreaterNoMatch", Condition{Kind: RoomMemberCountCondition, Is: ">2"}, `{}`, false},
		{"roomMemberCountGreaterMatch", Condition{Kind: RoomMemberCountCondition, Is: ">1"}, `{}`, true},

		{"senderNotificationPermissionMatch", Condition{Kind: SenderNotificationPermissionCondition, Key: "powerlevel"}, `{"sender":"@poweruser:example.com"}`, true},
		{"senderNotificationPermissionNoMatch", Condition{Kind: SenderNotificationPermissionCondition, Key: "powerlevel"}, `{"sender":"@nobody:example.com"}`, false},
	}
	for _, tst := range tsts {
		t.Run(tst.Name, func(t *testing.T) {
			got, err := conditionMatches(&tst.Cond, mustEventFromJSON(t, tst.EventJSON), &fakeEvaluationContext{2})
			if err != nil {
				t.Fatalf("conditionMatches failed: %v", err)
			}
			if got != tst.Want {
				t.Errorf("conditionMatches: got %v, want %v on %s", got, tst.Want, tst.Name)
			}
		})
	}
}

type fakeEvaluationContext struct{ memberCount int }

func (fakeEvaluationContext) UserDisplayName() string         { return "Dear User" }
func (f fakeEvaluationContext) RoomMemberCount() (int, error) { return f.memberCount, nil }
func (fakeEvaluationContext) HasPowerLevel(userID, levelKey string) (bool, error) {
	return userID == "@poweruser:example.com" && levelKey == "powerlevel", nil
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

func mustEventFromJSON(t *testing.T, json string) *gomatrixserverlib.Event {
	ev, err := gomatrixserverlib.NewEventFromTrustedJSON([]byte(json), false, gomatrixserverlib.RoomVersionV7)
	if err != nil {
		t.Fatal(err)
	}
	return ev
}
