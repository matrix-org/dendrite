package internal

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"time"

	"github.com/matrix-org/dendrite/internal/pushrules"
	"github.com/matrix-org/dendrite/pushserver/api"
	"github.com/matrix-org/dendrite/pushserver/producers"
	"github.com/matrix-org/dendrite/pushserver/storage"
	"github.com/matrix-org/dendrite/pushserver/storage/tables"
	"github.com/matrix-org/dendrite/setup/config"
	uapi "github.com/matrix-org/dendrite/userapi/api"
	"github.com/matrix-org/gomatrixserverlib"
	"github.com/matrix-org/util"
	"github.com/sirupsen/logrus"
)

// PushserverInternalAPI implements api.PushserverInternalAPI
type PushserverInternalAPI struct {
	Cfg          *config.PushServer
	DB           storage.Database
	userAPI      uapi.UserInternalAPI
	syncProducer *producers.SyncAPI
}

func NewPushserverAPI(
	cfg *config.PushServer, pushserverDB storage.Database, userAPI uapi.UserInternalAPI, syncProducer *producers.SyncAPI,
) *PushserverInternalAPI {
	a := &PushserverInternalAPI{
		Cfg:          cfg,
		DB:           pushserverDB,
		userAPI:      userAPI,
		syncProducer: syncProducer,
	}
	return a
}

func (a *PushserverInternalAPI) QueryNotifications(ctx context.Context, req *api.QueryNotificationsRequest, res *api.QueryNotificationsResponse) error {
	if req.Limit == 0 || req.Limit > 1000 {
		req.Limit = 1000
	}

	var fromID int64
	var err error
	if req.From != "" {
		fromID, err = strconv.ParseInt(req.From, 10, 64)
		if err != nil {
			return fmt.Errorf("QueryNotifications: parsing 'from': %w", err)
		}
	}
	var filter storage.NotificationFilter = tables.AllNotifications
	if req.Only == "highlight" {
		filter = tables.HighlightNotifications
	}
	notifs, lastID, err := a.DB.GetNotifications(ctx, req.Localpart, fromID, req.Limit, filter)
	if err != nil {
		return err
	}
	if notifs == nil {
		// This ensures empty is JSON-encoded as [] instead of null.
		notifs = []*api.Notification{}
	}
	res.Notifications = notifs
	if lastID >= 0 {
		res.NextToken = strconv.FormatInt(lastID+1, 10)
	}
	return nil
}

func (a *PushserverInternalAPI) PerformPusherSet(ctx context.Context, req *api.PerformPusherSetRequest, res *struct{}) error {
	util.GetLogger(ctx).WithFields(logrus.Fields{
		"localpart":    req.Localpart,
		"pushkey":      req.Pusher.PushKey,
		"display_name": req.Pusher.AppDisplayName,
	}).Info("PerformPusherCreation")
	if !req.Append {
		err := a.DB.RemovePushers(ctx, req.Pusher.AppID, req.Pusher.PushKey)
		if err != nil {
			return err
		}
	}
	if req.Pusher.Kind == "" {
		return a.DB.RemovePusher(ctx, req.Pusher.AppID, req.Pusher.PushKey, req.Localpart)
	}
	if req.Pusher.PushKeyTS == 0 {
		req.Pusher.PushKeyTS = gomatrixserverlib.AsTimestamp(time.Now())
	}
	return a.DB.UpsertPusher(ctx, req.Pusher, req.Localpart)
}

func (a *PushserverInternalAPI) PerformPusherDeletion(ctx context.Context, req *api.PerformPusherDeletionRequest, res *struct{}) error {
	pushers, err := a.DB.GetPushers(ctx, req.Localpart)
	if err != nil {
		return err
	}
	for i := range pushers {
		logrus.Warnf("pusher session: %d, req session: %d", pushers[i].SessionID, req.SessionID)
		if pushers[i].SessionID != req.SessionID {
			err := a.DB.RemovePusher(ctx, pushers[i].AppID, pushers[i].PushKey, req.Localpart)
			if err != nil {
				return err
			}
		}
	}
	return nil
}

func (a *PushserverInternalAPI) QueryPushers(ctx context.Context, req *api.QueryPushersRequest, res *api.QueryPushersResponse) error {
	var err error
	res.Pushers, err = a.DB.GetPushers(ctx, req.Localpart)
	return err
}

func (a *PushserverInternalAPI) PerformPushRulesPut(
	ctx context.Context,
	req *api.PerformPushRulesPutRequest,
	_ *struct{},
) error {
	bs, err := json.Marshal(&req.RuleSets)
	if err != nil {
		return err
	}
	userReq := uapi.InputAccountDataRequest{
		UserID:      req.UserID,
		DataType:    pushRulesAccountDataType,
		AccountData: json.RawMessage(bs),
	}
	var userRes uapi.InputAccountDataResponse // empty
	if err := a.userAPI.InputAccountData(ctx, &userReq, &userRes); err != nil {
		return err
	}

	if err := a.syncProducer.SendAccountData(req.UserID, "" /* roomID */, pushRulesAccountDataType); err != nil {
		util.GetLogger(ctx).WithError(err).Errorf("syncProducer.SendData failed")
	}

	return nil
}

func (a *PushserverInternalAPI) QueryPushRules(ctx context.Context, req *api.QueryPushRulesRequest, res *api.QueryPushRulesResponse) error {
	userReq := uapi.QueryAccountDataRequest{
		UserID:   req.UserID,
		DataType: pushRulesAccountDataType,
	}
	var userRes uapi.QueryAccountDataResponse
	if err := a.userAPI.QueryAccountData(ctx, &userReq, &userRes); err != nil {
		return err
	}
	bs, ok := userRes.GlobalAccountData[pushRulesAccountDataType]
	if !ok {
		// TODO: should this return the default rules? The default
		// rules are written to accounts DB on account creation, so
		// this error is unexpected.
		return fmt.Errorf("push rules account data not found")
	}
	var data pushrules.AccountRuleSets
	if err := json.Unmarshal([]byte(bs), &data); err != nil {
		util.GetLogger(ctx).WithError(err).Error("json.Unmarshal of push rules failed")
		return err
	}
	res.RuleSets = &data
	return nil
}

const pushRulesAccountDataType = "m.push_rules"
