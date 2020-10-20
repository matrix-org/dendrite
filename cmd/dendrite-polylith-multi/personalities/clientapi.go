package personalities

import (
	"github.com/matrix-org/dendrite/clientapi"
	"github.com/matrix-org/dendrite/internal/config"
	"github.com/matrix-org/dendrite/internal/setup"
	"github.com/matrix-org/dendrite/internal/transactions"
)

func ClientAPI(base *setup.BaseDendrite, cfg *config.Dendrite) {
	accountDB := base.CreateAccountsDB()
	federation := base.CreateFederationClient()

	asQuery := base.AppserviceHTTPClient()
	rsAPI := base.RoomserverHTTPClient()
	fsAPI := base.FederationSenderHTTPClient()
	eduInputAPI := base.EDUServerClient()
	userAPI := base.UserAPIClient()
	keyAPI := base.KeyServerHTTPClient()

	clientapi.AddPublicRoutes(
		base.PublicClientAPIMux, &base.Cfg.ClientAPI, accountDB, federation,
		rsAPI, eduInputAPI, asQuery, transactions.New(), fsAPI, userAPI, keyAPI, nil,
	)

	base.SetupAndServeHTTP(
		base.Cfg.ClientAPI.InternalAPI.Listen,
		base.Cfg.ClientAPI.ExternalAPI.Listen,
		nil, nil,
	)
}
