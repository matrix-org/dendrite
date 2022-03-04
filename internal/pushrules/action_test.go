package pushrules

import (
	"encoding/json"
	"fmt"
	"testing"

	"github.com/google/go-cmp/cmp"
)

func TestActionJSON(t *testing.T) {
	tsts := []struct {
		Want Action
	}{
		{Action{Kind: NotifyAction}},
		{Action{Kind: DontNotifyAction}},
		{Action{Kind: CoalesceAction}},
		{Action{Kind: SetTweakAction}},

		{Action{Kind: SetTweakAction, Tweak: SoundTweak, Value: "default"}},
		{Action{Kind: SetTweakAction, Tweak: HighlightTweak}},
		{Action{Kind: SetTweakAction, Tweak: HighlightTweak, Value: "false"}},
	}
	for _, tst := range tsts {
		t.Run(fmt.Sprintf("%+v", tst.Want), func(t *testing.T) {
			bs, err := json.Marshal(&tst.Want)
			if err != nil {
				t.Fatalf("Marshal failed: %v", err)
			}
			var got Action
			if err := json.Unmarshal(bs, &got); err != nil {
				t.Fatalf("Unmarshal failed: %v", err)
			}
			if diff := cmp.Diff(tst.Want, got); diff != "" {
				t.Errorf("+got -want:\n%s", diff)
			}
		})
	}
}
