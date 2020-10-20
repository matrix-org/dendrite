package personalities

import (
	"github.com/matrix-org/dendrite/federationsender"
	"github.com/matrix-org/dendrite/internal/config"
	"github.com/matrix-org/dendrite/internal/setup"
)

func FederationSender(base *setup.BaseDendrite, cfg *config.Dendrite) {
	federation := base.CreateFederationClient()

	serverKeyAPI := base.SigningKeyServerHTTPClient()
	keyRing := serverKeyAPI.KeyRing()

	rsAPI := base.RoomserverHTTPClient()
	fsAPI := federationsender.NewInternalAPI(
		base, federation, rsAPI, keyRing,
	)
	federationsender.AddInternalRoutes(base.InternalAPIMux, fsAPI)

	base.SetupAndServeHTTP(
		base.Cfg.FederationSender.InternalAPI.Listen, // internal listener
		setup.NoListener, // external listener
		nil, nil,
	)
}
