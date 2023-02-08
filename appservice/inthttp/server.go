package inthttp

import (
	"github.com/gorilla/mux"

	"github.com/matrix-org/dendrite/appservice/api"
	"github.com/matrix-org/dendrite/internal/httputil"
)

// AddRoutes adds the AppServiceQueryAPI handlers to the http.ServeMux.
func AddRoutes(a api.AppServiceInternalAPI, internalAPIMux *mux.Router) {
	internalAPIMux.Handle(
		AppServiceRoomAliasExistsPath,
		httputil.MakeInternalRPCAPI("AppserviceRoomAliasExists", a.RoomAliasExists),
	)

	internalAPIMux.Handle(
		AppServiceUserIDExistsPath,
		httputil.MakeInternalRPCAPI("AppserviceUserIDExists", a.UserIDExists),
	)

	internalAPIMux.Handle(
		AppServiceProtocolsPath,
		httputil.MakeInternalRPCAPI("AppserviceProtocols", a.Protocols),
	)

	internalAPIMux.Handle(
		AppServiceLocationsPath,
		httputil.MakeInternalRPCAPI("AppserviceLocations", a.Locations),
	)

	internalAPIMux.Handle(
		AppServiceUserPath,
		httputil.MakeInternalRPCAPI("AppserviceUser", a.User),
	)
}
