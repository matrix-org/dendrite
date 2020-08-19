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
	"go.uber.org/atomic"
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
	if stats.Blacklisted() {
		return stats, &api.FederationClientError{
			Blacklisted: true,
		}
	}
	// Call BackoffIfRequired with a closed channel to make it return immediately.
	// It will return the duration to backoff for.
	var duration time.Duration
	interrupt := make(chan bool)
	close(interrupt)
	var bo atomic.Bool
	duration, _ = stats.BackoffIfRequired(bo, interrupt)
	if duration > 0 {
		return stats, &api.FederationClientError{
			RetryAfter: duration,
		}
	}

	return stats, nil
}

func failBlacklistableError(err error, stats *statistics.ServerStatistics) {
	if err == nil {
		return
	}
	mxerr, ok := err.(gomatrix.HTTPError)
	if !ok {
		stats.Failure()
		return
	}
	if mxerr.Code == 500 {
		stats.Failure()
		return
	}
}

func (a *FederationSenderInternalAPI) GetUserDevices(
	ctx context.Context, s gomatrixserverlib.ServerName, userID string,
) (gomatrixserverlib.RespUserDevices, error) {
	var res gomatrixserverlib.RespUserDevices
	stats, err := a.isBlacklistedOrBackingOff(s)
	if err != nil {
		return res, err
	}
	res, err = a.federation.GetUserDevices(ctx, s, userID)
	if err != nil {
		failBlacklistableError(err, stats)
		return res, &api.FederationClientError{
			Err: err.Error(),
		}
	}
	stats.Success()
	return res, nil
}

func (a *FederationSenderInternalAPI) ClaimKeys(
	ctx context.Context, s gomatrixserverlib.ServerName, oneTimeKeys map[string]map[string]string,
) (gomatrixserverlib.RespClaimKeys, error) {
	var res gomatrixserverlib.RespClaimKeys
	stats, err := a.isBlacklistedOrBackingOff(s)
	if err != nil {
		return res, err
	}
	res, err = a.federation.ClaimKeys(ctx, s, oneTimeKeys)
	if err != nil {
		failBlacklistableError(err, stats)
		return res, &api.FederationClientError{
			Err: err.Error(),
		}
	}
	stats.Success()
	return res, nil
}

func (a *FederationSenderInternalAPI) QueryKeys(
	ctx context.Context, s gomatrixserverlib.ServerName, keys map[string][]string,
) (gomatrixserverlib.RespQueryKeys, error) {
	var res gomatrixserverlib.RespQueryKeys
	stats, err := a.isBlacklistedOrBackingOff(s)
	if err != nil {
		return res, err
	}
	res, err = a.federation.QueryKeys(ctx, s, keys)
	if err != nil {
		failBlacklistableError(err, stats)
		return res, &api.FederationClientError{
			Err: err.Error(),
		}
	}
	stats.Success()
	return res, nil
}
