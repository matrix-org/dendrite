package internal

import (
	"context"
	"time"

	"github.com/matrix-org/dendrite/federationsender/api"
	"github.com/matrix-org/dendrite/federationsender/queue"
	"github.com/matrix-org/dendrite/federationsender/statistics"
	"github.com/matrix-org/dendrite/federationsender/storage"
	"github.com/matrix-org/dendrite/internal/config"
	roomserverAPI "github.com/matrix-org/dendrite/roomserver/api"
	"github.com/matrix-org/gomatrix"
	"github.com/matrix-org/gomatrixserverlib"
	"github.com/matrix-org/util"
)

// FederationSenderInternalAPI is an implementation of api.FederationSenderInternalAPI
type FederationSenderInternalAPI struct {
	db         storage.Database
	cfg        *config.FederationSender
	statistics *statistics.Statistics
	rsAPI      roomserverAPI.RoomserverInternalAPI
	federation *gomatrixserverlib.FederationClient
	keyRing    *gomatrixserverlib.KeyRing
	queues     *queue.OutgoingQueues
}

func NewFederationSenderInternalAPI(
	db storage.Database, cfg *config.FederationSender,
	rsAPI roomserverAPI.RoomserverInternalAPI,
	federation *gomatrixserverlib.FederationClient,
	keyRing *gomatrixserverlib.KeyRing,
	statistics *statistics.Statistics,
	queues *queue.OutgoingQueues,
) *FederationSenderInternalAPI {
	return &FederationSenderInternalAPI{
		db:         db,
		cfg:        cfg,
		rsAPI:      rsAPI,
		federation: federation,
		keyRing:    keyRing,
		statistics: statistics,
		queues:     queues,
	}
}

func (a *FederationSenderInternalAPI) isBlacklistedOrBackingOff(s gomatrixserverlib.ServerName) (*statistics.ServerStatistics, error) {
	stats := a.statistics.ForServer(s)
	until, blacklisted := stats.BackoffInfo()
	if blacklisted {
		return stats, &api.FederationClientError{
			Blacklisted: true,
		}
	}
	now := time.Now()
	if until != nil && now.Before(*until) {
		return stats, &api.FederationClientError{
			RetryAfter: until.Sub(now),
		}
	}

	return stats, nil
}

func failBlacklistableError(err error, stats *statistics.ServerStatistics) (until time.Time, blacklisted bool) {
	if err == nil {
		return
	}
	mxerr, ok := err.(gomatrix.HTTPError)
	if !ok {
		return stats.Failure()
	}
	if mxerr.Code >= 500 && mxerr.Code < 600 {
		return stats.Failure()
	}
	return
}

func (a *FederationSenderInternalAPI) doRequest(
	ctx context.Context, s gomatrixserverlib.ServerName, request func() (interface{}, error),
) (interface{}, error) {
	stats, err := a.isBlacklistedOrBackingOff(s)
	if err != nil {
		util.GetLogger(ctx).Infof("isBlacklistedOrBackingOff %v", err)
		return nil, err
	}
	res, err := request()
	if err != nil {
		until, blacklisted := failBlacklistableError(err, stats)
		now := time.Now()
		var retryAfter time.Duration
		if until.After(now) {
			retryAfter = until.Sub(now)
		}
		return res, &api.FederationClientError{
			Err:         err.Error(),
			Blacklisted: blacklisted,
			RetryAfter:  retryAfter,
		}
	}
	stats.Success()
	return res, nil
}

func (a *FederationSenderInternalAPI) GetUserDevices(
	ctx context.Context, s gomatrixserverlib.ServerName, userID string,
) (gomatrixserverlib.RespUserDevices, error) {
	ires, err := a.doRequest(ctx, s, func() (interface{}, error) {
		util.GetLogger(ctx).Infof("GetUserDevices being called now")
		return a.federation.GetUserDevices(ctx, s, userID)
	})
	if err != nil {
		return gomatrixserverlib.RespUserDevices{}, err
	}
	return ires.(gomatrixserverlib.RespUserDevices), nil
}

func (a *FederationSenderInternalAPI) ClaimKeys(
	ctx context.Context, s gomatrixserverlib.ServerName, oneTimeKeys map[string]map[string]string,
) (gomatrixserverlib.RespClaimKeys, error) {
	ires, err := a.doRequest(ctx, s, func() (interface{}, error) {
		return a.federation.ClaimKeys(ctx, s, oneTimeKeys)
	})
	if err != nil {
		return gomatrixserverlib.RespClaimKeys{}, err
	}
	return ires.(gomatrixserverlib.RespClaimKeys), nil
}

func (a *FederationSenderInternalAPI) QueryKeys(
	ctx context.Context, s gomatrixserverlib.ServerName, keys map[string][]string,
) (gomatrixserverlib.RespQueryKeys, error) {
	ires, err := a.doRequest(ctx, s, func() (interface{}, error) {
		return a.federation.QueryKeys(ctx, s, keys)
	})
	if err != nil {
		return gomatrixserverlib.RespQueryKeys{}, err
	}
	return ires.(gomatrixserverlib.RespQueryKeys), nil
}
