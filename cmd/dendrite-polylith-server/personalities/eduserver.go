package personalities

import (
	"github.com/matrix-org/dendrite/eduserver"
	"github.com/matrix-org/dendrite/eduserver/cache"
	"github.com/matrix-org/dendrite/internal/config"
	"github.com/matrix-org/dendrite/internal/setup"
)

func EDUServer(base *setup.BaseDendrite, cfg *config.Dendrite) {
	intAPI := eduserver.NewInternalAPI(base, cache.New(), base.UserAPIClient())
	eduserver.AddInternalRoutes(base.InternalAPIMux, intAPI)

	base.SetupAndServeHTTP(
		base.Cfg.EDUServer.InternalAPI.Listen, // internal listener
		setup.NoListener,                      // external listener
		nil, nil,
	)
}
