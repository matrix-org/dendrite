package personalities

import (
	"github.com/matrix-org/dendrite/internal/config"
	"github.com/matrix-org/dendrite/internal/setup"
	"github.com/matrix-org/dendrite/keyserver"
)

func KeyServer(base *setup.BaseDendrite, cfg *config.Dendrite) {
	intAPI := keyserver.NewInternalAPI(&base.Cfg.KeyServer, base.CreateFederationClient())
	intAPI.SetUserAPI(base.UserAPIClient())

	keyserver.AddInternalRoutes(base.InternalAPIMux, intAPI)

	base.SetupAndServeHTTP(
		base.Cfg.KeyServer.InternalAPI.Listen, // internal listener
		setup.NoListener,                      // external listener
		nil, nil,
	)
}
