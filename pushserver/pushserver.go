package pushserver

import (
	"github.com/gorilla/mux"
	"github.com/matrix-org/dendrite/pushserver/api"
	"github.com/matrix-org/dendrite/pushserver/internal"
	"github.com/matrix-org/dendrite/pushserver/inthttp"
	"github.com/matrix-org/dendrite/pushserver/storage"
	roomserverAPI "github.com/matrix-org/dendrite/roomserver/api"
	"github.com/matrix-org/dendrite/setup/config"
	"github.com/sirupsen/logrus"
)

// AddInternalRoutes registers HTTP handlers for the internal API. Invokes functions
// on the given input API.
func AddInternalRoutes(router *mux.Router, intAPI api.PushserverInternalAPI) {
	inthttp.AddRoutes(intAPI, router)
}

// NewInternalAPI returns a concerete implementation of the internal API. Callers
// can call functions directly on the returned API or via an HTTP interface using AddInternalRoutes.
func NewInternalAPI(
	cfg *config.PushServer,
	rsAPI roomserverAPI.RoomserverInternalAPI,
) api.PushserverInternalAPI {
	db, err := storage.Open(&cfg.Database)
	if err != nil {
		logrus.WithError(err).Panicf("failed to connect to push server db")
	}

	psAPI := internal.NewPushserverAPI(
		cfg, db,
	)

	return psAPI
}
