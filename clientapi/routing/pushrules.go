package routing

import (
	"encoding/json"
	"net/http"

	"github.com/matrix-org/dendrite/clientapi/auth/authtypes"
	"github.com/matrix-org/dendrite/clientapi/auth/storage/accounts"
	"github.com/matrix-org/dendrite/clientapi/jsonerror"
	"github.com/matrix-org/gomatrixserverlib"
	"github.com/matrix-org/util"
)

type PushCondition struct {
	Kind    string `json:"kind"`
	Key     string `json:"key"`
	Pattern string `json:"pattern"`
	Is      string `json:"is"`
}

type PushRule struct {
	Actions    interface{}     `json:"actions"`
	Default    bool            `json:"default"`
	Enabled    bool            `json:"enabled"`
	RuleId     string          `json:"rule_id"`
	Conditions []PushCondition `json:"conditions"`
	Pattern    string          `json:"pattern"`
}
type Global struct {
	Content   []PushRule `json:"content"`
	Override  []PushRule `json:"override"`
	Room      []PushRule `json:"room"`
	Sender    []PushRule `json:"sender"`
	Underride []PushRule `json:"underride"`
}
type RuleSet struct {
	Global Global `json:"global"`
}

func GetPushRuleSet(
	req *http.Request,
	dev *authtypes.Device,
	accountDB accounts.Database,
) util.JSONResponse {
	localpart, _, err := gomatrixserverlib.SplitID('@', dev.UserID)
	if err != nil {
		util.GetLogger(req.Context()).WithError(err).Error("gomatrixserverlib.SplitID failed")
		return jsonerror.InternalServerError()
	}
	data, err := accountDB.GetAccountDataByType(req.Context(), localpart, "", "m.push_rules")
	if err != nil {
		util.GetLogger(req.Context()).WithError(err).Error("m.push_rules data not found")
		return jsonerror.InternalServerError()
	}
	pushRuleSet := RuleSet{Global{
		Content:   []PushRule{},
		Override:  []PushRule{},
		Room:      []PushRule{},
		Sender:    []PushRule{},
		Underride: []PushRule{},
	}}

	err = json.Unmarshal(data.Content, &pushRuleSet)
	if err != nil {
		util.GetLogger(req.Context()).WithError(err).Error("Could not unmarshal pushrules data")
		return jsonerror.InternalServerError()
	}

	return util.JSONResponse{
		Code: http.StatusOK,
		JSON: pushRuleSet,
	}
}

func GetPushRule(
	req *http.Request, dev *authtypes.Device, accountDB accounts.Database,
	kind string, ruleID string,
) util.JSONResponse {
	localpart, _, err := gomatrixserverlib.SplitID('@', dev.UserID)
	if err != nil {
		util.GetLogger(req.Context()).WithError(err).Error("gomatrixserverlib.SplitID failed")
		return jsonerror.InternalServerError()
	}
	data, err := accountDB.GetAccountDataByType(req.Context(), localpart, "", "m.push_rules")
	if err != nil {
		util.GetLogger(req.Context()).WithError(err).Error("m.push_rules data not found")
		return jsonerror.InternalServerError()
	}
	pushRuleSet := RuleSet{Global{
		Content:   []PushRule{},
		Override:  []PushRule{},
		Room:      []PushRule{},
		Sender:    []PushRule{},
		Underride: []PushRule{},
	}}

	err = json.Unmarshal(data.Content, &pushRuleSet)
	if err != nil {
		util.GetLogger(req.Context()).WithError(err).Error("Could not unmarshal pushrules data")
		return jsonerror.InternalServerError()
	}
	pushRule := PushRule{}
	switch kind {
	case "override":
		pushRule = getPushRulebyId(pushRuleSet.Global.Override, ruleID)

	case "underride":
		pushRule = getPushRulebyId(pushRuleSet.Global.Underride, ruleID)

	case "sender":
		pushRule = getPushRulebyId(pushRuleSet.Global.Sender, ruleID)

	case "room":
		pushRule = getPushRulebyId(pushRuleSet.Global.Room, ruleID)

	case "content":
		pushRule = getPushRulebyId(pushRuleSet.Global.Content, ruleID)

	default:
		return util.JSONResponse{
			Code: http.StatusBadRequest,
			JSON: jsonerror.NotFound("Unrecognised request"),
		}
	}
	if pushRule.RuleId == ruleID {
		return util.JSONResponse{
			Code: http.StatusOK,
			JSON: pushRule,
		}
	}
	return util.JSONResponse{
		Code: http.StatusNotFound,
		JSON: jsonerror.NotFound("Not found"),
	}
}

//Returns a single push rule which matches given ruleID
func getPushRulebyId(pushRules []PushRule, ruleID string) PushRule {
	for _, pushRule := range pushRules {
		if pushRule.RuleId == ruleID {
			return pushRule
		}
	}
	return PushRule{}
}
