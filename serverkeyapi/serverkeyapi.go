package serverkeyapi

import (
	"crypto/ed25519"
	"encoding/base64"

	"github.com/matrix-org/dendrite/internal/basecomponent"
	"github.com/matrix-org/dendrite/serverkeyapi/api"
	"github.com/matrix-org/dendrite/serverkeyapi/internal"
	"github.com/matrix-org/dendrite/serverkeyapi/storage"
	"github.com/matrix-org/dendrite/serverkeyapi/storage/cache"
	"github.com/matrix-org/gomatrixserverlib"
	"github.com/sirupsen/logrus"
)

func SetupServerKeyAPIComponent(
	base *basecomponent.BaseDendrite,
	fedClient *gomatrixserverlib.FederationClient,
) api.ServerKeyInternalAPI {
	innerDB, err := storage.NewDatabase(
		string(base.Cfg.Database.ServerKey),
		base.Cfg.DbProperties(),
		base.Cfg.Matrix.ServerName,
		base.Cfg.Matrix.PrivateKey.Public().(ed25519.PublicKey),
		base.Cfg.Matrix.KeyID,
	)
	if err != nil {
		logrus.WithError(err).Panicf("failed to connect to server key database")
	}

	serverKeyDB, err := cache.NewKeyDatabase(innerDB, base.ImmutableCache)
	if err != nil {
		logrus.WithError(err).Panicf("failed to set up caching wrapper for server key database")
	}

	internalAPI := internal.ServerKeyAPI{
		DB:             serverKeyDB,
		Cfg:            base.Cfg,
		ImmutableCache: base.ImmutableCache,
		FedClient:      fedClient,
		OurKeyRing: gomatrixserverlib.KeyRing{
			KeyFetchers: []gomatrixserverlib.KeyFetcher{
				&gomatrixserverlib.DirectKeyFetcher{
					Client: fedClient.Client,
				},
			},
			KeyDatabase: serverKeyDB,
		},
	}

	var b64e = base64.StdEncoding.WithPadding(base64.NoPadding)
	for _, ps := range base.Cfg.Matrix.KeyPerspectives {
		perspective := &gomatrixserverlib.PerspectiveKeyFetcher{
			PerspectiveServerName: ps.ServerName,
			PerspectiveServerKeys: map[gomatrixserverlib.KeyID]ed25519.PublicKey{},
			Client:                fedClient.Client,
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

	internalAPI.SetupHTTP(base.InternalAPIMux)

	return &internalAPI
}
