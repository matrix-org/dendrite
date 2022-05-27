package internal

import (
	"crypto/ed25519"
	"encoding/base64"
	"fmt"
	"sync"
	"time"

	"github.com/matrix-org/dendrite/federationapi/api"
	"github.com/matrix-org/dendrite/federationapi/queue"
	"github.com/matrix-org/dendrite/federationapi/statistics"
	"github.com/matrix-org/dendrite/federationapi/storage"
	"github.com/matrix-org/dendrite/federationapi/storage/cache"
	"github.com/matrix-org/dendrite/internal/caching"
	roomserverAPI "github.com/matrix-org/dendrite/roomserver/api"
	"github.com/matrix-org/dendrite/setup/config"
	"github.com/matrix-org/gomatrix"
	"github.com/matrix-org/gomatrixserverlib"
	"github.com/sirupsen/logrus"
)

// FederationInternalAPI is an implementation of api.FederationInternalAPI
type FederationInternalAPI struct {
	db         storage.Database
	cfg        *config.FederationAPI
	statistics *statistics.Statistics
	rsAPI      roomserverAPI.FederationRoomserverAPI
	federation api.FederationClient
	keyRing    *gomatrixserverlib.KeyRing
	queues     *queue.OutgoingQueues
	joins      sync.Map // joins currently in progress
}

func NewFederationInternalAPI(
	db storage.Database, cfg *config.FederationAPI,
	rsAPI roomserverAPI.FederationRoomserverAPI,
	federation api.FederationClient,
	statistics *statistics.Statistics,
	caches *caching.Caches,
	queues *queue.OutgoingQueues,
	keyRing *gomatrixserverlib.KeyRing,
) *FederationInternalAPI {
	serverKeyDB, err := cache.NewKeyDatabase(db, caches)
	if err != nil {
		logrus.WithError(err).Panicf("failed to set up caching wrapper for server key database")
	}

	if keyRing == nil {
		keyRing = &gomatrixserverlib.KeyRing{
			KeyFetchers: []gomatrixserverlib.KeyFetcher{},
			KeyDatabase: serverKeyDB,
		}

		addDirectFetcher := func() {
			keyRing.KeyFetchers = append(
				keyRing.KeyFetchers,
				&gomatrixserverlib.DirectKeyFetcher{
					Client: federation,
				},
			)
		}

		if cfg.PreferDirectFetch {
			addDirectFetcher()
		} else {
			defer addDirectFetcher()
		}

		var b64e = base64.StdEncoding.WithPadding(base64.NoPadding)
		for _, ps := range cfg.KeyPerspectives {
			perspective := &gomatrixserverlib.PerspectiveKeyFetcher{
				PerspectiveServerName: ps.ServerName,
				PerspectiveServerKeys: map[gomatrixserverlib.KeyID]ed25519.PublicKey{},
				Client:                federation,
			}

			for _, key := range ps.Keys {
				rawkey, err := b64e.DecodeString(key.PublicKey)
				if err != nil {
					logrus.WithError(err).WithFields(logrus.Fields{
						"server_name": ps.ServerName,
						"public_key":  key.PublicKey,
					}).Warn("Couldn't parse perspective key")
					continue
				}
				perspective.PerspectiveServerKeys[key.KeyID] = rawkey
			}

			keyRing.KeyFetchers = append(keyRing.KeyFetchers, perspective)

			logrus.WithFields(logrus.Fields{
				"server_name":     ps.ServerName,
				"num_public_keys": len(ps.Keys),
			}).Info("Enabled perspective key fetcher")
		}
	}

	return &FederationInternalAPI{
		db:         db,
		cfg:        cfg,
		rsAPI:      rsAPI,
		keyRing:    keyRing,
		federation: federation,
		statistics: statistics,
		queues:     queues,
	}
}

func (a *FederationInternalAPI) isBlacklistedOrBackingOff(s gomatrixserverlib.ServerName) (*statistics.ServerStatistics, error) {
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

func (a *FederationInternalAPI) doRequestIfNotBackingOffOrBlacklisted(
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

func (a *FederationInternalAPI) doRequestIfNotBlacklisted(
	s gomatrixserverlib.ServerName, request func() (interface{}, error),
) (interface{}, error) {
	stats := a.statistics.ForServer(s)
	if _, blacklisted := stats.BackoffInfo(); blacklisted {
		return stats, &api.FederationClientError{
			Err:         fmt.Sprintf("server %q is blacklisted", s),
			Blacklisted: true,
		}
	}
	return request()
}
