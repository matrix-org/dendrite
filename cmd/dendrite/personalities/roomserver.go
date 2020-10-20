package personalities

import (
	"github.com/matrix-org/dendrite/internal/config"
	"github.com/matrix-org/dendrite/internal/setup"
	"github.com/matrix-org/dendrite/roomserver"
)

func RoomServer(base *setup.BaseDendrite, cfg *config.Dendrite) {
	serverKeyAPI := base.SigningKeyServerHTTPClient()
	keyRing := serverKeyAPI.KeyRing()

	fsAPI := base.FederationSenderHTTPClient()
	rsAPI := roomserver.NewInternalAPI(base, keyRing)
	rsAPI.SetFederationSenderAPI(fsAPI)
	roomserver.AddInternalRoutes(base.InternalAPIMux, rsAPI)

	base.SetupAndServeHTTP(
		base.Cfg.RoomServer.InternalAPI.Listen, // internal listener
		setup.NoListener,                       // external listener
		nil, nil,
	)
}
