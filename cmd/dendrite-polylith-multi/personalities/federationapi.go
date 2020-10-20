package personalities

import (
	"github.com/matrix-org/dendrite/federationapi"
	"github.com/matrix-org/dendrite/internal/config"
	"github.com/matrix-org/dendrite/internal/setup"
)

func FederationAPI(base *setup.BaseDendrite, cfg *config.Dendrite) {
	userAPI := base.UserAPIClient()
	federation := base.CreateFederationClient()
	serverKeyAPI := base.SigningKeyServerHTTPClient()
	keyRing := serverKeyAPI.KeyRing()
	fsAPI := base.FederationSenderHTTPClient()
	rsAPI := base.RoomserverHTTPClient()
	keyAPI := base.KeyServerHTTPClient()

	federationapi.AddPublicRoutes(
		base.PublicFederationAPIMux, base.PublicKeyAPIMux,
		&base.Cfg.FederationAPI, userAPI, federation, keyRing,
		rsAPI, fsAPI, base.EDUServerClient(), keyAPI,
	)

	base.SetupAndServeHTTP(
		base.Cfg.FederationAPI.InternalAPI.Listen,
		base.Cfg.FederationAPI.ExternalAPI.Listen,
		nil, nil,
	)
}
