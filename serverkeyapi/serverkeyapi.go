package serverkeyapi

import (
	"crypto/ed25519"
	"encoding/base64"

	"github.com/gorilla/mux"
	"github.com/matrix-org/dendrite/internal/caching"
	"github.com/matrix-org/dendrite/internal/config"
	"github.com/matrix-org/dendrite/serverkeyapi/api"
	"github.com/matrix-org/dendrite/serverkeyapi/internal"
	"github.com/matrix-org/dendrite/serverkeyapi/inthttp"
	"github.com/matrix-org/dendrite/serverkeyapi/storage"
	"github.com/matrix-org/dendrite/serverkeyapi/storage/cache"
	"github.com/matrix-org/gomatrixserverlib"
	"github.com/sirupsen/logrus"
)

// AddInternalRoutes registers HTTP handlers for the internal API. Invokes functions
// on the given input API.
func AddInternalRoutes(router *mux.Router, intAPI api.ServerKeyInternalAPI, caches *caching.Caches) {
	inthttp.AddRoutes(intAPI, router, caches)
}

// NewInternalAPI returns a concerete implementation of the internal API. Callers
// can call functions directly on the returned API or via an HTTP interface using AddInternalRoutes.
func NewInternalAPI(
	cfg *config.ServerKeyAPI,
	fedClient gomatrixserverlib.KeyClient,
	caches *caching.Caches,
) api.ServerKeyInternalAPI {
	innerDB, err := storage.NewDatabase(
		&cfg.Database,
		cfg.Matrix.ServerName,
		cfg.Matrix.PrivateKey.Public().(ed25519.PublicKey),
		cfg.Matrix.KeyID,
	)
	if err != nil {
		logrus.WithError(err).Panicf("failed to connect to server key database")
	}

	serverKeyDB, err := cache.NewKeyDatabase(innerDB, caches)
	if err != nil {
		logrus.WithError(err).Panicf("failed to set up caching wrapper for server key database")
	}

	internalAPI := internal.ServerKeyAPI{
		ServerName:        cfg.Matrix.ServerName,
		ServerPublicKey:   cfg.Matrix.PrivateKey.Public().(ed25519.PublicKey),
		ServerKeyID:       cfg.Matrix.KeyID,
		ServerKeyValidity: cfg.Matrix.KeyValidityPeriod,
		OldServerKeys:     cfg.Matrix.OldVerifyKeys,
		FedClient:         fedClient,
		OurKeyRing: gomatrixserverlib.KeyRing{
			KeyFetchers: []gomatrixserverlib.KeyFetcher{},
			KeyDatabase: serverKeyDB,
		},
	}

	addDirectFetcher := func() {
		internalAPI.OurKeyRing.KeyFetchers = append(
			internalAPI.OurKeyRing.KeyFetchers,
			&gomatrixserverlib.DirectKeyFetcher{
				Client: fedClient,
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
			Client:                fedClient,
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

		internalAPI.OurKeyRing.KeyFetchers = append(
			internalAPI.OurKeyRing.KeyFetchers,
			perspective,
		)

		logrus.WithFields(logrus.Fields{
			"server_name":     ps.ServerName,
			"num_public_keys": len(ps.Keys),
		}).Info("Enabled perspective key fetcher")
	}

	return &internalAPI
}
