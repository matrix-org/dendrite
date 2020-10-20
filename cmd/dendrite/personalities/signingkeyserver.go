package personalities

import (
	"github.com/matrix-org/dendrite/internal/config"
	"github.com/matrix-org/dendrite/internal/setup"
	"github.com/matrix-org/dendrite/signingkeyserver"
)

func SigningKeyServer(base *setup.BaseDendrite, cfg *config.Dendrite) {
	federation := base.CreateFederationClient()

	intAPI := signingkeyserver.NewInternalAPI(&base.Cfg.SigningKeyServer, federation, base.Caches)
	signingkeyserver.AddInternalRoutes(base.InternalAPIMux, intAPI, base.Caches)

	base.SetupAndServeHTTP(
		base.Cfg.SigningKeyServer.InternalAPI.Listen,
		setup.NoListener,
		nil, nil,
	)
}
