package routing

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"reflect"

	"github.com/matrix-org/dendrite/clientapi/jsonerror"
	"github.com/matrix-org/dendrite/internal/pushrules"
	userapi "github.com/matrix-org/dendrite/userapi/api"
	"github.com/matrix-org/util"
)

func errorResponse(ctx context.Context, err error, msg string, args ...interface{}) util.JSONResponse {
	if eerr, ok := err.(*jsonerror.MatrixError); ok {
		var status int
		switch eerr.ErrCode {
		case "M_INVALID_ARGUMENT_VALUE":
			status = http.StatusBadRequest
		case "M_NOT_FOUND":
			status = http.StatusNotFound
		default:
			status = http.StatusInternalServerError
		}
		return util.MatrixErrorResponse(status, eerr.ErrCode, eerr.Err)
	}
	util.GetLogger(ctx).WithError(err).Errorf(msg, args...)
	return jsonerror.InternalServerError()
}

func GetAllPushRules(ctx context.Context, device *userapi.Device, userAPI userapi.UserInternalAPI) util.JSONResponse {
	ruleSets, err := queryPushRules(ctx, device.UserID, userAPI)
	if err != nil {
		return errorResponse(ctx, err, "queryPushRulesJSON failed")
	}
	return util.JSONResponse{
		Code: http.StatusOK,
		JSON: ruleSets,
	}
}

func GetPushRulesByScope(ctx context.Context, scope string, device *userapi.Device, userAPI userapi.UserInternalAPI) util.JSONResponse {
	ruleSets, err := queryPushRules(ctx, device.UserID, userAPI)
	if err != nil {
		return errorResponse(ctx, err, "queryPushRulesJSON failed")
	}
	ruleSet := pushRuleSetByScope(ruleSets, pushrules.Scope(scope))
	if ruleSet == nil {
		return errorResponse(ctx, jsonerror.InvalidArgumentValue("invalid push rule set"), "pushRuleSetByScope failed")
	}
	return util.JSONResponse{
		Code: http.StatusOK,
		JSON: ruleSet,
	}
}

func GetPushRulesByKind(ctx context.Context, scope, kind string, device *userapi.Device, userAPI userapi.UserInternalAPI) util.JSONResponse {
	ruleSets, err := queryPushRules(ctx, device.UserID, userAPI)
	if err != nil {
		return errorResponse(ctx, err, "queryPushRules failed")
	}
	ruleSet := pushRuleSetByScope(ruleSets, pushrules.Scope(scope))
	if ruleSet == nil {
		return errorResponse(ctx, jsonerror.InvalidArgumentValue("invalid push rule set"), "pushRuleSetByScope failed")
	}
	rulesPtr := pushRuleSetKindPointer(ruleSet, pushrules.Kind(kind))
	if rulesPtr == nil {
		return errorResponse(ctx, jsonerror.InvalidArgumentValue("invalid push rules kind"), "pushRuleSetKindPointer failed")
	}
	return util.JSONResponse{
		Code: http.StatusOK,
		JSON: *rulesPtr,
	}
}

func GetPushRuleByRuleID(ctx context.Context, scope, kind, ruleID string, device *userapi.Device, userAPI userapi.UserInternalAPI) util.JSONResponse {
	ruleSets, err := queryPushRules(ctx, device.UserID, userAPI)
	if err != nil {
		return errorResponse(ctx, err, "queryPushRules failed")
	}
	ruleSet := pushRuleSetByScope(ruleSets, pushrules.Scope(scope))
	if ruleSet == nil {
		return errorResponse(ctx, jsonerror.InvalidArgumentValue("invalid push rule set"), "pushRuleSetByScope failed")
	}
	rulesPtr := pushRuleSetKindPointer(ruleSet, pushrules.Kind(kind))
	if rulesPtr == nil {
		return errorResponse(ctx, jsonerror.InvalidArgumentValue("invalid push rules kind"), "pushRuleSetKindPointer failed")
	}
	i := pushRuleIndexByID(*rulesPtr, ruleID)
	if i < 0 {
		return errorResponse(ctx, jsonerror.NotFound("push rule ID not found"), "pushRuleIndexByID failed")
	}
	return util.JSONResponse{
		Code: http.StatusOK,
		JSON: (*rulesPtr)[i],
	}
}

func PutPushRuleByRuleID(ctx context.Context, scope, kind, ruleID, afterRuleID, beforeRuleID string, body io.Reader, device *userapi.Device, userAPI userapi.UserInternalAPI) util.JSONResponse {
	var newRule pushrules.Rule
	if err := json.NewDecoder(body).Decode(&newRule); err != nil {
		return errorResponse(ctx, err, "JSON Decode failed")
	}
	newRule.RuleID = ruleID

	errs := pushrules.ValidateRule(pushrules.Kind(kind), &newRule)
	if len(errs) > 0 {
		return errorResponse(ctx, jsonerror.InvalidArgumentValue(errs[0].Error()), "rule sanity check failed: %v", errs)
	}

	ruleSets, err := queryPushRules(ctx, device.UserID, userAPI)
	if err != nil {
		return errorResponse(ctx, err, "queryPushRules failed")
	}
	ruleSet := pushRuleSetByScope(ruleSets, pushrules.Scope(scope))
	if ruleSet == nil {
		return errorResponse(ctx, jsonerror.InvalidArgumentValue("invalid push rule set"), "pushRuleSetByScope failed")
	}
	rulesPtr := pushRuleSetKindPointer(ruleSet, pushrules.Kind(kind))
	if rulesPtr == nil {
		return errorResponse(ctx, jsonerror.InvalidArgumentValue("invalid push rules kind"), "pushRuleSetKindPointer failed")
	}
	i := pushRuleIndexByID(*rulesPtr, ruleID)
	if i >= 0 && afterRuleID == "" && beforeRuleID == "" {
		// Modify rule at the same index.

		// TODO: The spec does not say what to do in this case, but
		// this feels reasonable.
		*((*rulesPtr)[i]) = newRule
		util.GetLogger(ctx).Infof("Modified existing push rule at %d", i)
	} else {
		if i >= 0 {
			// Delete old rule.
			*rulesPtr = append((*rulesPtr)[:i], (*rulesPtr)[i+1:]...)
			util.GetLogger(ctx).Infof("Deleted old push rule at %d", i)
		} else {
			// SPEC: When creating push rules, they MUST be enabled by default.
			//
			// TODO: it's unclear if we must reject disabled rules, or force
			// the value to true. Sytests fail if we don't force it.
			newRule.Enabled = true
		}

		// Add new rule.
		i, err := findPushRuleInsertionIndex(*rulesPtr, afterRuleID, beforeRuleID)
		if err != nil {
			return errorResponse(ctx, err, "findPushRuleInsertionIndex failed")
		}

		*rulesPtr = append((*rulesPtr)[:i], append([]*pushrules.Rule{&newRule}, (*rulesPtr)[i:]...)...)
		util.GetLogger(ctx).WithField("after", afterRuleID).WithField("before", beforeRuleID).Infof("Added new push rule at %d", i)
	}

	if err := putPushRules(ctx, device.UserID, ruleSets, userAPI); err != nil {
		return errorResponse(ctx, err, "putPushRules failed")
	}

	return util.JSONResponse{Code: http.StatusOK, JSON: struct{}{}}
}

func DeletePushRuleByRuleID(ctx context.Context, scope, kind, ruleID string, device *userapi.Device, userAPI userapi.UserInternalAPI) util.JSONResponse {
	ruleSets, err := queryPushRules(ctx, device.UserID, userAPI)
	if err != nil {
		return errorResponse(ctx, err, "queryPushRules failed")
	}
	ruleSet := pushRuleSetByScope(ruleSets, pushrules.Scope(scope))
	if ruleSet == nil {
		return errorResponse(ctx, jsonerror.InvalidArgumentValue("invalid push rule set"), "pushRuleSetByScope failed")
	}
	rulesPtr := pushRuleSetKindPointer(ruleSet, pushrules.Kind(kind))
	if rulesPtr == nil {
		return errorResponse(ctx, jsonerror.InvalidArgumentValue("invalid push rules kind"), "pushRuleSetKindPointer failed")
	}
	i := pushRuleIndexByID(*rulesPtr, ruleID)
	if i < 0 {
		return errorResponse(ctx, jsonerror.NotFound("push rule ID not found"), "pushRuleIndexByID failed")
	}

	*rulesPtr = append((*rulesPtr)[:i], (*rulesPtr)[i+1:]...)

	if err := putPushRules(ctx, device.UserID, ruleSets, userAPI); err != nil {
		return errorResponse(ctx, err, "putPushRules failed")
	}

	return util.JSONResponse{Code: http.StatusOK, JSON: struct{}{}}
}

func GetPushRuleAttrByRuleID(ctx context.Context, scope, kind, ruleID, attr string, device *userapi.Device, userAPI userapi.UserInternalAPI) util.JSONResponse {
	attrGet, err := pushRuleAttrGetter(attr)
	if err != nil {
		return errorResponse(ctx, err, "pushRuleAttrGetter failed")
	}
	ruleSets, err := queryPushRules(ctx, device.UserID, userAPI)
	if err != nil {
		return errorResponse(ctx, err, "queryPushRules failed")
	}
	ruleSet := pushRuleSetByScope(ruleSets, pushrules.Scope(scope))
	if ruleSet == nil {
		return errorResponse(ctx, jsonerror.InvalidArgumentValue("invalid push rule set"), "pushRuleSetByScope failed")
	}
	rulesPtr := pushRuleSetKindPointer(ruleSet, pushrules.Kind(kind))
	if rulesPtr == nil {
		return errorResponse(ctx, jsonerror.InvalidArgumentValue("invalid push rules kind"), "pushRuleSetKindPointer failed")
	}
	i := pushRuleIndexByID(*rulesPtr, ruleID)
	if i < 0 {
		return errorResponse(ctx, jsonerror.NotFound("push rule ID not found"), "pushRuleIndexByID failed")
	}
	return util.JSONResponse{
		Code: http.StatusOK,
		JSON: map[string]interface{}{
			attr: attrGet((*rulesPtr)[i]),
		},
	}
}

func PutPushRuleAttrByRuleID(ctx context.Context, scope, kind, ruleID, attr string, body io.Reader, device *userapi.Device, userAPI userapi.UserInternalAPI) util.JSONResponse {
	var newPartialRule pushrules.Rule
	if err := json.NewDecoder(body).Decode(&newPartialRule); err != nil {
		return util.JSONResponse{
			Code: http.StatusBadRequest,
			JSON: jsonerror.BadJSON(err.Error()),
		}
	}
	if newPartialRule.Actions == nil {
		// This ensures json.Marshal encodes the empty list as [] rather than null.
		newPartialRule.Actions = []*pushrules.Action{}
	}

	attrGet, err := pushRuleAttrGetter(attr)
	if err != nil {
		return errorResponse(ctx, err, "pushRuleAttrGetter failed")
	}
	attrSet, err := pushRuleAttrSetter(attr)
	if err != nil {
		return errorResponse(ctx, err, "pushRuleAttrSetter failed")
	}

	ruleSets, err := queryPushRules(ctx, device.UserID, userAPI)
	if err != nil {
		return errorResponse(ctx, err, "queryPushRules failed")
	}
	ruleSet := pushRuleSetByScope(ruleSets, pushrules.Scope(scope))
	if ruleSet == nil {
		return errorResponse(ctx, jsonerror.InvalidArgumentValue("invalid push rule set"), "pushRuleSetByScope failed")
	}
	rulesPtr := pushRuleSetKindPointer(ruleSet, pushrules.Kind(kind))
	if rulesPtr == nil {
		return errorResponse(ctx, jsonerror.InvalidArgumentValue("invalid push rules kind"), "pushRuleSetKindPointer failed")
	}
	i := pushRuleIndexByID(*rulesPtr, ruleID)
	if i < 0 {
		return errorResponse(ctx, jsonerror.NotFound("push rule ID not found"), "pushRuleIndexByID failed")
	}

	if !reflect.DeepEqual(attrGet((*rulesPtr)[i]), attrGet(&newPartialRule)) {
		attrSet((*rulesPtr)[i], &newPartialRule)

		if err := putPushRules(ctx, device.UserID, ruleSets, userAPI); err != nil {
			return errorResponse(ctx, err, "putPushRules failed")
		}
	}

	return util.JSONResponse{Code: http.StatusOK, JSON: struct{}{}}
}

func queryPushRules(ctx context.Context, userID string, userAPI userapi.UserInternalAPI) (*pushrules.AccountRuleSets, error) {
	var res userapi.QueryPushRulesResponse
	if err := userAPI.QueryPushRules(ctx, &userapi.QueryPushRulesRequest{UserID: userID}, &res); err != nil {
		util.GetLogger(ctx).WithError(err).Error("userAPI.QueryPushRules failed")
		return nil, err
	}
	return res.RuleSets, nil
}

func putPushRules(ctx context.Context, userID string, ruleSets *pushrules.AccountRuleSets, userAPI userapi.UserInternalAPI) error {
	req := userapi.PerformPushRulesPutRequest{
		UserID:   userID,
		RuleSets: ruleSets,
	}
	var res struct{}
	if err := userAPI.PerformPushRulesPut(ctx, &req, &res); err != nil {
		util.GetLogger(ctx).WithError(err).Error("userAPI.PerformPushRulesPut failed")
		return err
	}
	return nil
}

func pushRuleSetByScope(ruleSets *pushrules.AccountRuleSets, scope pushrules.Scope) *pushrules.RuleSet {
	switch scope {
	case pushrules.GlobalScope:
		return &ruleSets.Global
	default:
		return nil
	}
}

func pushRuleSetKindPointer(ruleSet *pushrules.RuleSet, kind pushrules.Kind) *[]*pushrules.Rule {
	switch kind {
	case pushrules.OverrideKind:
		return &ruleSet.Override
	case pushrules.ContentKind:
		return &ruleSet.Content
	case pushrules.RoomKind:
		return &ruleSet.Room
	case pushrules.SenderKind:
		return &ruleSet.Sender
	case pushrules.UnderrideKind:
		return &ruleSet.Underride
	default:
		return nil
	}
}

func pushRuleIndexByID(rules []*pushrules.Rule, id string) int {
	for i, rule := range rules {
		if rule.RuleID == id {
			return i
		}
	}
	return -1
}

func pushRuleAttrGetter(attr string) (func(*pushrules.Rule) interface{}, error) {
	switch attr {
	case "actions":
		return func(rule *pushrules.Rule) interface{} { return rule.Actions }, nil
	case "enabled":
		return func(rule *pushrules.Rule) interface{} { return rule.Enabled }, nil
	default:
		return nil, jsonerror.InvalidArgumentValue("invalid push rule attribute")
	}
}

func pushRuleAttrSetter(attr string) (func(dest, src *pushrules.Rule), error) {
	switch attr {
	case "actions":
		return func(dest, src *pushrules.Rule) { dest.Actions = src.Actions }, nil
	case "enabled":
		return func(dest, src *pushrules.Rule) { dest.Enabled = src.Enabled }, nil
	default:
		return nil, jsonerror.InvalidArgumentValue("invalid push rule attribute")
	}
}

func findPushRuleInsertionIndex(rules []*pushrules.Rule, afterID, beforeID string) (int, error) {
	var i int

	if afterID != "" {
		for ; i < len(rules); i++ {
			if rules[i].RuleID == afterID {
				break
			}
		}
		if i == len(rules) {
			return 0, jsonerror.NotFound("after: rule ID not found")
		}
		if rules[i].Default {
			return 0, jsonerror.NotFound("after: rule ID must not be a default rule")
		}
		// We stopped on the "after" match to differentiate
		// not-found from is-last-entry. Now we move to the earliest
		// insertion point.
		i++
	}

	if beforeID != "" {
		for ; i < len(rules); i++ {
			if rules[i].RuleID == beforeID {
				break
			}
		}
		if i == len(rules) {
			return 0, jsonerror.NotFound("before: rule ID not found")
		}
		if rules[i].Default {
			return 0, jsonerror.NotFound("before: rule ID must not be a default rule")
		}
	}

	// UNSPEC: The spec does not say what to do if no after/before is
	// given. Sytest fails if it doesn't go first.
	return i, nil
}
