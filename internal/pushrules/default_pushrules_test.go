package pushrules

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
)

// Tests that the pre-defined rules as of
// https://spec.matrix.org/v1.4/client-server-api/#predefined-rules
// are correct
func TestDefaultRules(t *testing.T) {
	type testCase struct {
		name       string
		inputBytes []byte
		want       Rule
	}

	testCases := []testCase{
		// Default override rules
		{
			name:       ".m.rule.master",
			inputBytes: []byte(`{"rule_id":".m.rule.master","default":true,"enabled":false,"actions":[]}`),
			want:       mRuleMasterDefinition,
		},
		{
			name:       ".m.rule.suppress_notices",
			inputBytes: []byte(`{"rule_id":".m.rule.suppress_notices","default":true,"enabled":true,"conditions":[{"kind":"event_match","key":"content.msgtype","pattern":"m.notice"}],"actions":[]}`),
			want:       mRuleSuppressNoticesDefinition,
		},
		{
			name:       ".m.rule.invite_for_me",
			inputBytes: []byte(`{"rule_id":".m.rule.invite_for_me","default":true,"enabled":true,"conditions":[{"kind":"event_match","key":"type","pattern":"m.room.member"},{"kind":"event_match","key":"content.membership","pattern":"invite"},{"kind":"event_match","key":"state_key","pattern":"@test:localhost"}],"actions":["notify",{"set_tweak":"sound","value":"default"}]}`),
			want:       *mRuleInviteForMeDefinition("@test:localhost"),
		},
		{
			name:       ".m.rule.member_event",
			inputBytes: []byte(`{"rule_id":".m.rule.member_event","default":true,"enabled":true,"conditions":[{"kind":"event_match","key":"type","pattern":"m.room.member"}],"actions":[]}`),
			want:       mRuleMemberEventDefinition,
		},
		{
			name:       ".m.rule.contains_display_name",
			inputBytes: []byte(`{"rule_id":".m.rule.contains_display_name","default":true,"enabled":true,"conditions":[{"kind":"contains_display_name"}],"actions":["notify",{"set_tweak":"sound","value":"default"},{"set_tweak":"highlight"}]}`),
			want:       mRuleContainsDisplayNameDefinition,
		},
		{
			name:       ".m.rule.tombstone",
			inputBytes: []byte(`{"rule_id":".m.rule.tombstone","default":true,"enabled":true,"conditions":[{"kind":"event_match","key":"type","pattern":"m.room.tombstone"},{"kind":"event_match","key":"state_key","pattern":""}],"actions":["notify",{"set_tweak":"highlight"}]}`),
			want:       mRuleTombstoneDefinition,
		},
		{
			name:       ".m.rule.room.server_acl",
			inputBytes: []byte(`{"rule_id":".m.rule.room.server_acl","default":true,"enabled":true,"conditions":[{"kind":"event_match","key":"type","pattern":"m.room.server_acl"},{"kind":"event_match","key":"state_key","pattern":""}],"actions":[]}`),
			want:       mRuleACLsDefinition,
		},
		{
			name:       ".m.rule.roomnotif",
			inputBytes: []byte(`{"rule_id":".m.rule.roomnotif","default":true,"enabled":true,"conditions":[{"kind":"event_match","key":"content.body","pattern":"@room"},{"kind":"sender_notification_permission","key":"room"}],"actions":["notify",{"set_tweak":"highlight"}]}`),
			want:       mRuleRoomNotifDefinition,
		},
		// Default content rules
		{
			name:       ".m.rule.contains_user_name",
			inputBytes: []byte(`{"rule_id":".m.rule.contains_user_name","default":true,"enabled":true,"actions":["notify",{"set_tweak":"sound","value":"default"},{"set_tweak":"highlight"}],"pattern":"myLocalUser"}`),
			want:       *mRuleContainsUserNameDefinition("myLocalUser"),
		},
		// default underride rules
		{
			name:       ".m.rule.call",
			inputBytes: []byte(`{"rule_id":".m.rule.call","default":true,"enabled":true,"conditions":[{"kind":"event_match","key":"type","pattern":"m.call.invite"}],"actions":["notify",{"set_tweak":"sound","value":"ring"}]}`),
			want:       mRuleCallDefinition,
		},
		{
			name:       ".m.rule.encrypted_room_one_to_one",
			inputBytes: []byte(`{"rule_id":".m.rule.encrypted_room_one_to_one","default":true,"enabled":true,"conditions":[{"kind":"room_member_count","is":"2"},{"kind":"event_match","key":"type","pattern":"m.room.encrypted"}],"actions":["notify",{"set_tweak":"sound","value":"default"}]}`),
			want:       mRuleEncryptedRoomOneToOneDefinition,
		},
		{
			name:       ".m.rule.room_one_to_one",
			inputBytes: []byte(`{"rule_id":".m.rule.room_one_to_one","default":true,"enabled":true,"conditions":[{"kind":"room_member_count","is":"2"},{"kind":"event_match","key":"type","pattern":"m.room.message"}],"actions":["notify",{"set_tweak":"sound","value":"default"}]}`),
			want:       mRuleRoomOneToOneDefinition,
		},
		{
			name:       ".m.rule.message",
			inputBytes: []byte(`{"rule_id":".m.rule.message","default":true,"enabled":true,"conditions":[{"kind":"event_match","key":"type","pattern":"m.room.message"}],"actions":["notify"]}`),
			want:       mRuleMessageDefinition,
		},
		{
			name:       ".m.rule.encrypted",
			inputBytes: []byte(`{"rule_id":".m.rule.encrypted","default":true,"enabled":true,"conditions":[{"kind":"event_match","key":"type","pattern":"m.room.encrypted"}],"actions":["notify"]}`),
			want:       mRuleEncryptedDefinition,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			r := Rule{}
			// unmarshal predefined push rules
			err := json.Unmarshal(tc.inputBytes, &r)
			assert.NoError(t, err)
			assert.Equal(t, tc.want, r)

			// and reverse it to check we get the expected result
			got, err := json.Marshal(r)
			assert.NoError(t, err)
			assert.Equal(t, string(got), string(tc.inputBytes))
		})

	}
}
