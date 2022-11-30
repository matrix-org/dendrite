package pushrules

import (
	"fmt"
	"testing"

	"github.com/google/go-cmp/cmp"
)

func TestActionsToTweaks(t *testing.T) {
	tsts := []struct {
		Name       string
		Input      []*Action
		WantKind   ActionKind
		WantTweaks map[string]interface{}
	}{
		{"empty", nil, UnknownAction, nil},
		{"zero", []*Action{{}}, UnknownAction, nil},
		{"onlyPrimary", []*Action{{Kind: NotifyAction}}, NotifyAction, nil},
		{"onlyPrimaryDontNotify", []*Action{{Kind: DontNotifyAction}}, DontNotifyAction, nil},
		{"onlyTweak", []*Action{{Kind: SetTweakAction, Tweak: HighlightTweak}}, UnknownAction, map[string]interface{}{"highlight": nil}},
		{"onlyTweakWithValue", []*Action{{Kind: SetTweakAction, Tweak: SoundTweak, Value: "default"}}, UnknownAction, map[string]interface{}{"sound": "default"}},
		{
			"all",
			[]*Action{
				{Kind: CoalesceAction},
				{Kind: SetTweakAction, Tweak: HighlightTweak},
				{Kind: SetTweakAction, Tweak: SoundTweak, Value: "default"},
			},
			CoalesceAction,
			map[string]interface{}{"highlight": nil, "sound": "default"},
		},
	}
	for _, tst := range tsts {
		t.Run(tst.Name, func(t *testing.T) {
			gotKind, gotTweaks, err := ActionsToTweaks(tst.Input)
			if err != nil {
				t.Fatalf("ActionsToTweaks failed: %v", err)
			}
			if gotKind != tst.WantKind {
				t.Errorf("kind: got %v, want %v", gotKind, tst.WantKind)
			}
			if diff := cmp.Diff(tst.WantTweaks, gotTweaks); diff != "" {
				t.Errorf("tweaks: +got -want:\n%s", diff)
			}
		})
	}
}

func TestBoolTweakOr(t *testing.T) {
	tsts := []struct {
		Name  string
		Input map[string]interface{}
		Def   bool
		Want  bool
	}{
		{"nil", nil, false, false},
		{"nilValue", map[string]interface{}{"highlight": nil}, true, true},
		{"false", map[string]interface{}{"highlight": false}, true, false},
		{"true", map[string]interface{}{"highlight": true}, false, true},
	}
	for _, tst := range tsts {
		t.Run(tst.Name, func(t *testing.T) {
			got := BoolTweakOr(tst.Input, HighlightTweak, tst.Def)
			if got != tst.Want {
				t.Errorf("BoolTweakOr: got %v, want %v", got, tst.Want)
			}
		})
	}
}

func TestGlobToRegexp(t *testing.T) {
	tsts := []struct {
		Input string
		Want  string
	}{
		{"", "^(.*.*)$"},
		{"a", "^(.*a.*)$"},
		{"a.b", "^(.*a\\.b.*)$"},
		{"a?b", "^(a.b)$"},
		{"a*b*", "^(a.*b.*)$"},
		{"a*b?", "^(a.*b.)$"},
	}
	for _, tst := range tsts {
		t.Run(tst.Want, func(t *testing.T) {
			got, err := globToRegexp(tst.Input)
			if err != nil {
				t.Fatalf("globToRegexp failed: %v", err)
			}
			if got.String() != tst.Want {
				t.Errorf("got %v, want %v", got.String(), tst.Want)
			}
		})
	}
}

func TestLookupMapPath(t *testing.T) {
	tsts := []struct {
		Path []string
		Root map[string]interface{}
		Want interface{}
	}{
		{[]string{"a"}, map[string]interface{}{"a": "b"}, "b"},
		{[]string{"a"}, map[string]interface{}{"a": 42}, 42},
		{[]string{"a", "b"}, map[string]interface{}{"a": map[string]interface{}{"b": "c"}}, "c"},
	}
	for _, tst := range tsts {
		t.Run(fmt.Sprint(tst.Path, "/", tst.Want), func(t *testing.T) {
			got, err := lookupMapPath(tst.Path, tst.Root)
			if err != nil {
				t.Fatalf("lookupMapPath failed: %v", err)
			}
			if diff := cmp.Diff(tst.Want, got); diff != "" {
				t.Errorf("+got -want:\n%s", diff)
			}
		})
	}
}

func TestLookupMapPathInvalid(t *testing.T) {
	tsts := []struct {
		Path []string
		Root map[string]interface{}
	}{
		{nil, nil},
		{[]string{"a"}, nil},
		{[]string{"a", "b"}, map[string]interface{}{"a": "c"}},
	}
	for _, tst := range tsts {
		t.Run(fmt.Sprint(tst.Path), func(t *testing.T) {
			got, err := lookupMapPath(tst.Path, tst.Root)
			if err == nil {
				t.Fatalf("lookupMapPath succeeded with %#v, but want failure", got)
			}
		})
	}
}

func TestParseRoomMemberCountCondition(t *testing.T) {
	tsts := []struct {
		Input     string
		WantTrue  []int
		WantFalse []int
	}{
		{"1", []int{1}, []int{0, 2}},
		{"==1", []int{1}, []int{0, 2}},
		{"<1", []int{0}, []int{1, 2}},
		{"<=1", []int{0, 1}, []int{2}},
		{">1", []int{2}, []int{0, 1}},
		{">=42", []int{42, 43}, []int{41}},
	}
	for _, tst := range tsts {
		t.Run(tst.Input, func(t *testing.T) {
			got, err := parseRoomMemberCountCondition(tst.Input)
			if err != nil {
				t.Fatalf("parseRoomMemberCountCondition failed: %v", err)
			}
			for _, v := range tst.WantTrue {
				if !got(v) {
					t.Errorf("parseRoomMemberCountCondition(%q)(%d): got false, want true", tst.Input, v)
				}
			}
			for _, v := range tst.WantFalse {
				if got(v) {
					t.Errorf("parseRoomMemberCountCondition(%q)(%d): got true, want false", tst.Input, v)
				}
			}
		})
	}
}
