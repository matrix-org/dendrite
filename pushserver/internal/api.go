package internal

import (
	"context"
	"time"

	"github.com/matrix-org/dendrite/pushserver/api"
	"github.com/matrix-org/dendrite/pushserver/storage"
	"github.com/matrix-org/dendrite/setup/config"
	"github.com/matrix-org/gomatrixserverlib"
	"github.com/matrix-org/util"
	"github.com/sirupsen/logrus"
)

// PushserverInternalAPI implements api.PushserverInternalAPI
type PushserverInternalAPI struct {
	Cfg *config.PushServer
	DB  storage.Database
}

func NewPushserverAPI(
	cfg *config.PushServer, pushserverDB storage.Database,
) *PushserverInternalAPI {
	a := &PushserverInternalAPI{
		Cfg: cfg,
		DB:  pushserverDB,
	}
	return a
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
	return a.DB.CreatePusher(ctx, req.Pusher, req.Localpart)
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
