package internal

import (
	"context"
	"sync"
	"time"

	"github.com/matrix-org/dendrite/federationsender/api"
	"github.com/matrix-org/dendrite/federationsender/queue"
	"github.com/matrix-org/dendrite/federationsender/statistics"
	"github.com/matrix-org/dendrite/federationsender/storage"
	roomserverAPI "github.com/matrix-org/dendrite/roomserver/api"
	"github.com/matrix-org/dendrite/setup/config"
	"github.com/matrix-org/gomatrix"
	"github.com/matrix-org/gomatrixserverlib"
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
	joins      sync.Map // joins currently in progress
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
			RetryAfter: time.Until(*until),
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
	if mxerr.Code == 401 { // invalid signature in X-Matrix header
		return stats.Failure()
	}
	if mxerr.Code >= 500 && mxerr.Code < 600 { // internal server errors
		return stats.Failure()
	}
	return
}

func (a *FederationSenderInternalAPI) doRequest(
	s gomatrixserverlib.ServerName, request func() (interface{}, error),
) (interface{}, error) {
	stats, err := a.isBlacklistedOrBackingOff(s)
	if err != nil {
		return nil, err
	}
	res, err := request()
	if err != nil {
		until, blacklisted := failBlacklistableError(err, stats)
		now := time.Now()
		var retryAfter time.Duration
		if until.After(now) {
			retryAfter = time.Until(until)
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
	ctx, cancel := context.WithTimeout(ctx, time.Second*30)
	defer cancel()
	ires, err := a.doRequest(s, func() (interface{}, error) {
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
	ctx, cancel := context.WithTimeout(ctx, time.Second*30)
	defer cancel()
	ires, err := a.doRequest(s, func() (interface{}, error) {
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
	ires, err := a.doRequest(s, func() (interface{}, error) {
		return a.federation.QueryKeys(ctx, s, keys)
	})
	if err != nil {
		return gomatrixserverlib.RespQueryKeys{}, err
	}
	return ires.(gomatrixserverlib.RespQueryKeys), nil
}

func (a *FederationSenderInternalAPI) Backfill(
	ctx context.Context, s gomatrixserverlib.ServerName, roomID string, limit int, eventIDs []string,
) (res gomatrixserverlib.Transaction, err error) {
	ctx, cancel := context.WithTimeout(ctx, time.Second*30)
	defer cancel()
	ires, err := a.doRequest(s, func() (interface{}, error) {
		return a.federation.Backfill(ctx, s, roomID, limit, eventIDs)
	})
	if err != nil {
		return gomatrixserverlib.Transaction{}, err
	}
	return ires.(gomatrixserverlib.Transaction), nil
}

func (a *FederationSenderInternalAPI) LookupState(
	ctx context.Context, s gomatrixserverlib.ServerName, roomID, eventID string, roomVersion gomatrixserverlib.RoomVersion,
) (res gomatrixserverlib.RespState, err error) {
	ctx, cancel := context.WithTimeout(ctx, time.Second*30)
	defer cancel()
	ires, err := a.doRequest(s, func() (interface{}, error) {
		return a.federation.LookupState(ctx, s, roomID, eventID, roomVersion)
	})
	if err != nil {
		return gomatrixserverlib.RespState{}, err
	}
	return ires.(gomatrixserverlib.RespState), nil
}

func (a *FederationSenderInternalAPI) LookupStateIDs(
	ctx context.Context, s gomatrixserverlib.ServerName, roomID, eventID string,
) (res gomatrixserverlib.RespStateIDs, err error) {
	ctx, cancel := context.WithTimeout(ctx, time.Second*30)
	defer cancel()
	ires, err := a.doRequest(s, func() (interface{}, error) {
		return a.federation.LookupStateIDs(ctx, s, roomID, eventID)
	})
	if err != nil {
		return gomatrixserverlib.RespStateIDs{}, err
	}
	return ires.(gomatrixserverlib.RespStateIDs), nil
}

func (a *FederationSenderInternalAPI) GetEvent(
	ctx context.Context, s gomatrixserverlib.ServerName, eventID string,
) (res gomatrixserverlib.Transaction, err error) {
	ctx, cancel := context.WithTimeout(ctx, time.Second*30)
	defer cancel()
	ires, err := a.doRequest(s, func() (interface{}, error) {
		return a.federation.GetEvent(ctx, s, eventID)
	})
	if err != nil {
		return gomatrixserverlib.Transaction{}, err
	}
	return ires.(gomatrixserverlib.Transaction), nil
}

func (a *FederationSenderInternalAPI) GetServerKeys(
	ctx context.Context, s gomatrixserverlib.ServerName,
) (gomatrixserverlib.ServerKeys, error) {
	ctx, cancel := context.WithTimeout(ctx, time.Second*30)
	defer cancel()
	ires, err := a.doRequest(s, func() (interface{}, error) {
		return a.federation.GetServerKeys(ctx, s)
	})
	if err != nil {
		return gomatrixserverlib.ServerKeys{}, err
	}
	return ires.(gomatrixserverlib.ServerKeys), nil
}

func (a *FederationSenderInternalAPI) LookupServerKeys(
	ctx context.Context, s gomatrixserverlib.ServerName, keyRequests map[gomatrixserverlib.PublicKeyLookupRequest]gomatrixserverlib.Timestamp,
) ([]gomatrixserverlib.ServerKeys, error) {
	ctx, cancel := context.WithTimeout(ctx, time.Minute)
	defer cancel()
	ires, err := a.doRequest(s, func() (interface{}, error) {
		return a.federation.LookupServerKeys(ctx, s, keyRequests)
	})
	if err != nil {
		return []gomatrixserverlib.ServerKeys{}, err
	}
	return ires.([]gomatrixserverlib.ServerKeys), nil
}

func (a *FederationSenderInternalAPI) MSC2836EventRelationships(
	ctx context.Context, s gomatrixserverlib.ServerName, r gomatrixserverlib.MSC2836EventRelationshipsRequest,
	roomVersion gomatrixserverlib.RoomVersion,
) (res gomatrixserverlib.MSC2836EventRelationshipsResponse, err error) {
	ctx, cancel := context.WithTimeout(ctx, time.Minute)
	defer cancel()
	ires, err := a.doRequest(s, func() (interface{}, error) {
		return a.federation.MSC2836EventRelationships(ctx, s, r, roomVersion)
	})
	if err != nil {
		return res, err
	}
	return ires.(gomatrixserverlib.MSC2836EventRelationshipsResponse), nil
}

func (a *FederationSenderInternalAPI) MSC2946Spaces(
	ctx context.Context, s gomatrixserverlib.ServerName, roomID string, r gomatrixserverlib.MSC2946SpacesRequest,
) (res gomatrixserverlib.MSC2946SpacesResponse, err error) {
	ctx, cancel := context.WithTimeout(ctx, time.Minute)
	defer cancel()
	ires, err := a.doRequest(s, func() (interface{}, error) {
		return a.federation.MSC2946Spaces(ctx, s, roomID, r)
	})
	if err != nil {
		return res, err
	}
	return ires.(gomatrixserverlib.MSC2946SpacesResponse), nil
}
