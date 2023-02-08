package inthttp

import (
	"github.com/gorilla/mux"

	"github.com/matrix-org/dendrite/appservice/api"
	"github.com/matrix-org/dendrite/internal/httputil"
)

// AddRoutes adds the AppServiceQueryAPI handlers to the http.ServeMux.
func AddRoutes(a api.AppServiceInternalAPI, internalAPIMux *mux.Router, enableMetrics bool) {
	internalAPIMux.Handle(
		AppServiceRoomAliasExistsPath,
		httputil.MakeInternalRPCAPI("AppserviceRoomAliasExists", enableMetrics, a.RoomAliasExists),
	)

	internalAPIMux.Handle(
		AppServiceUserIDExistsPath,
		httputil.MakeInternalRPCAPI("AppserviceUserIDExists", enableMetrics, a.UserIDExists),
	)

	internalAPIMux.Handle(
		AppServiceProtocolsPath,
		httputil.MakeInternalRPCAPI("AppserviceProtocols", enableMetrics, a.Protocols),
	)

	internalAPIMux.Handle(
		AppServiceLocationsPath,
		httputil.MakeInternalRPCAPI("AppserviceLocations", enableMetrics, a.Locations),
	)

	internalAPIMux.Handle(
		AppServiceUserPath,
		httputil.MakeInternalRPCAPI("AppserviceUser", enableMetrics, a.User),
	)

}
