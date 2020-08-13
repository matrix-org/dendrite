// Copyright 2020 The Matrix.org Foundation C.I.C.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package setup

import (
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"

	currentstateAPI "github.com/matrix-org/dendrite/currentstateserver/api"
	"github.com/matrix-org/dendrite/internal/caching"
	"github.com/matrix-org/dendrite/internal/httputil"
	"github.com/matrix-org/dendrite/internal/sqlutil"
	"github.com/matrix-org/gomatrixserverlib"
	"github.com/matrix-org/naffka"
	"github.com/prometheus/client_golang/prometheus/promhttp"

	"github.com/matrix-org/dendrite/internal"
	"github.com/matrix-org/dendrite/userapi/storage/accounts"
	"github.com/matrix-org/dendrite/userapi/storage/devices"

	"github.com/Shopify/sarama"
	"github.com/gorilla/mux"

	appserviceAPI "github.com/matrix-org/dendrite/appservice/api"
	asinthttp "github.com/matrix-org/dendrite/appservice/inthttp"
	currentstateinthttp "github.com/matrix-org/dendrite/currentstateserver/inthttp"
	eduServerAPI "github.com/matrix-org/dendrite/eduserver/api"
	eduinthttp "github.com/matrix-org/dendrite/eduserver/inthttp"
	federationSenderAPI "github.com/matrix-org/dendrite/federationsender/api"
	fsinthttp "github.com/matrix-org/dendrite/federationsender/inthttp"
	"github.com/matrix-org/dendrite/internal/config"
	keyserverAPI "github.com/matrix-org/dendrite/keyserver/api"
	keyinthttp "github.com/matrix-org/dendrite/keyserver/inthttp"
	roomserverAPI "github.com/matrix-org/dendrite/roomserver/api"
	rsinthttp "github.com/matrix-org/dendrite/roomserver/inthttp"
	serverKeyAPI "github.com/matrix-org/dendrite/serverkeyapi/api"
	skinthttp "github.com/matrix-org/dendrite/serverkeyapi/inthttp"
	userapi "github.com/matrix-org/dendrite/userapi/api"
	userapiinthttp "github.com/matrix-org/dendrite/userapi/inthttp"
	"github.com/sirupsen/logrus"

	_ "net/http/pprof"
)

// BaseDendrite is a base for creating new instances of dendrite. It parses
// command line flags and config, and exposes methods for creating various
// resources. All errors are handled by logging then exiting, so all methods
// should only be used during start up.
// Must be closed when shutting down.
type BaseDendrite struct {
	componentName          string
	tracerCloser           io.Closer
	PublicClientAPIMux     *mux.Router
	PublicFederationAPIMux *mux.Router
	PublicKeyAPIMux        *mux.Router
	PublicMediaAPIMux      *mux.Router
	InternalAPIMux         *mux.Router
	UseHTTPAPIs            bool
	httpClient             *http.Client
	Cfg                    *config.Dendrite
	Caches                 *caching.Caches
	KafkaConsumer          sarama.Consumer
	KafkaProducer          sarama.SyncProducer
}

const HTTPServerTimeout = time.Minute * 5
const HTTPClientTimeout = time.Second * 30

const NoExternalListener = ""

// NewBaseDendrite creates a new instance to be used by a component.
// The componentName is used for logging purposes, and should be a friendly name
// of the compontent running, e.g. "SyncAPI"
func NewBaseDendrite(cfg *config.Dendrite, componentName string, useHTTPAPIs bool) *BaseDendrite {
	configErrors := &config.ConfigErrors{}
	cfg.Verify(configErrors, componentName == "Monolith") // TODO: better way?
	if len(*configErrors) > 0 {
		for _, err := range *configErrors {
			logrus.Errorf("Configuration error: %s", err)
		}
		logrus.Fatalf("Failed to start due to configuration errors")
	}

	internal.SetupStdLogging()
	internal.SetupHookLogging(cfg.Logging, componentName)
	internal.SetupPprof()

	closer, err := cfg.SetupTracing("Dendrite" + componentName)
	if err != nil {
		logrus.WithError(err).Panicf("failed to start opentracing")
	}

	var kafkaConsumer sarama.Consumer
	var kafkaProducer sarama.SyncProducer
	if cfg.Global.Kafka.UseNaffka {
		kafkaConsumer, kafkaProducer = setupNaffka(cfg)
	} else {
		kafkaConsumer, kafkaProducer = setupKafka(cfg)
	}

	cache, err := caching.NewInMemoryLRUCache(true)
	if err != nil {
		logrus.WithError(err).Warnf("Failed to create cache")
	}

	client := http.Client{Timeout: HTTPClientTimeout}
	if cfg.FederationSender.Proxy.Enabled {
		client.Transport = &http.Transport{Proxy: http.ProxyURL(&url.URL{
			Scheme: cfg.FederationSender.Proxy.Protocol,
			Host:   fmt.Sprintf("%s:%d", cfg.FederationSender.Proxy.Host, cfg.FederationSender.Proxy.Port),
		})}
	}

	// Ideally we would only use SkipClean on routes which we know can allow '/' but due to
	// https://github.com/gorilla/mux/issues/460 we have to attach this at the top router.
	// When used in conjunction with UseEncodedPath() we get the behaviour we want when parsing
	// path parameters:
	// /foo/bar%2Fbaz    == [foo, bar%2Fbaz]  (from UseEncodedPath)
	// /foo/bar%2F%2Fbaz == [foo, bar%2F%2Fbaz] (from SkipClean)
	// In particular, rooms v3 event IDs are not urlsafe and can include '/' and because they
	// are randomly generated it results in flakey tests.
	// We need to be careful with media APIs if they read from a filesystem to make sure they
	// are not inadvertently reading paths without cleaning, else this could introduce a
	// directory traversal attack e.g /../../../etc/passwd
	return &BaseDendrite{
		componentName:          componentName,
		UseHTTPAPIs:            useHTTPAPIs,
		tracerCloser:           closer,
		Cfg:                    cfg,
		Caches:                 cache,
		PublicClientAPIMux:     mux.NewRouter().SkipClean(true).PathPrefix(httputil.PublicClientPathPrefix).Subrouter().UseEncodedPath(),
		PublicFederationAPIMux: mux.NewRouter().SkipClean(true).PathPrefix(httputil.PublicFederationPathPrefix).Subrouter().UseEncodedPath(),
		PublicKeyAPIMux:        mux.NewRouter().SkipClean(true).PathPrefix(httputil.PublicKeyPathPrefix).Subrouter().UseEncodedPath(),
		PublicMediaAPIMux:      mux.NewRouter().SkipClean(true).PathPrefix(httputil.PublicMediaPathPrefix).Subrouter().UseEncodedPath(),
		InternalAPIMux:         mux.NewRouter().SkipClean(true).PathPrefix(httputil.InternalPathPrefix).Subrouter().UseEncodedPath(),
		httpClient:             &client,
		KafkaConsumer:          kafkaConsumer,
		KafkaProducer:          kafkaProducer,
	}
}

// Close implements io.Closer
func (b *BaseDendrite) Close() error {
	return b.tracerCloser.Close()
}

// AppserviceHTTPClient returns the AppServiceQueryAPI for hitting the appservice component over HTTP.
func (b *BaseDendrite) AppserviceHTTPClient() appserviceAPI.AppServiceQueryAPI {
	a, err := asinthttp.NewAppserviceClient(b.Cfg.AppServiceURL(), b.httpClient)
	if err != nil {
		logrus.WithError(err).Panic("CreateHTTPAppServiceAPIs failed")
	}
	return a
}

// RoomserverHTTPClient returns RoomserverInternalAPI for hitting the roomserver over HTTP.
func (b *BaseDendrite) RoomserverHTTPClient() roomserverAPI.RoomserverInternalAPI {
	rsAPI, err := rsinthttp.NewRoomserverClient(b.Cfg.RoomServerURL(), b.httpClient, b.Caches)
	if err != nil {
		logrus.WithError(err).Panic("RoomserverHTTPClient failed", b.httpClient)
	}
	return rsAPI
}

// UserAPIClient returns UserInternalAPI for hitting the userapi over HTTP.
func (b *BaseDendrite) UserAPIClient() userapi.UserInternalAPI {
	userAPI, err := userapiinthttp.NewUserAPIClient(b.Cfg.UserAPIURL(), b.httpClient)
	if err != nil {
		logrus.WithError(err).Panic("UserAPIClient failed", b.httpClient)
	}
	return userAPI
}

// CurrentStateAPIClient returns CurrentStateInternalAPI for hitting the currentstateserver over HTTP.
func (b *BaseDendrite) CurrentStateAPIClient() currentstateAPI.CurrentStateInternalAPI {
	stateAPI, err := currentstateinthttp.NewCurrentStateAPIClient(b.Cfg.CurrentStateAPIURL(), b.httpClient)
	if err != nil {
		logrus.WithError(err).Panic("UserAPIClient failed", b.httpClient)
	}
	return stateAPI
}

// EDUServerClient returns EDUServerInputAPI for hitting the EDU server over HTTP
func (b *BaseDendrite) EDUServerClient() eduServerAPI.EDUServerInputAPI {
	e, err := eduinthttp.NewEDUServerClient(b.Cfg.EDUServerURL(), b.httpClient)
	if err != nil {
		logrus.WithError(err).Panic("EDUServerClient failed", b.httpClient)
	}
	return e
}

// FederationSenderHTTPClient returns FederationSenderInternalAPI for hitting
// the federation sender over HTTP
func (b *BaseDendrite) FederationSenderHTTPClient() federationSenderAPI.FederationSenderInternalAPI {
	f, err := fsinthttp.NewFederationSenderClient(b.Cfg.FederationSenderURL(), b.httpClient)
	if err != nil {
		logrus.WithError(err).Panic("FederationSenderHTTPClient failed", b.httpClient)
	}
	return f
}

// ServerKeyAPIClient returns ServerKeyInternalAPI for hitting the server key API over HTTP
func (b *BaseDendrite) ServerKeyAPIClient() serverKeyAPI.ServerKeyInternalAPI {
	f, err := skinthttp.NewServerKeyClient(
		b.Cfg.ServerKeyAPIURL(),
		b.httpClient,
		b.Caches,
	)
	if err != nil {
		logrus.WithError(err).Panic("NewServerKeyInternalAPIHTTP failed", b.httpClient)
	}
	return f
}

// KeyServerHTTPClient returns KeyInternalAPI for hitting the key server over HTTP
func (b *BaseDendrite) KeyServerHTTPClient() keyserverAPI.KeyInternalAPI {
	f, err := keyinthttp.NewKeyServerClient(b.Cfg.KeyServerURL(), b.httpClient)
	if err != nil {
		logrus.WithError(err).Panic("KeyServerHTTPClient failed", b.httpClient)
	}
	return f
}

// CreateDeviceDB creates a new instance of the device database. Should only be
// called once per component.
func (b *BaseDendrite) CreateDeviceDB() devices.Database {
	db, err := devices.NewDatabase(&b.Cfg.UserAPI.DeviceDatabase, b.Cfg.Global.ServerName)
	if err != nil {
		logrus.WithError(err).Panicf("failed to connect to devices db")
	}

	return db
}

// CreateAccountsDB creates a new instance of the accounts database. Should only
// be called once per component.
func (b *BaseDendrite) CreateAccountsDB() accounts.Database {
	db, err := accounts.NewDatabase(&b.Cfg.UserAPI.AccountDatabase, b.Cfg.Global.ServerName)
	if err != nil {
		logrus.WithError(err).Panicf("failed to connect to accounts db")
	}

	return db
}

// CreateFederationClient creates a new federation client. Should only be called
// once per component.
func (b *BaseDendrite) CreateFederationClient() *gomatrixserverlib.FederationClient {
	return gomatrixserverlib.NewFederationClient(
		b.Cfg.Global.ServerName, b.Cfg.Global.KeyID, b.Cfg.Global.PrivateKey,
		b.Cfg.FederationSender.DisableTLSValidation,
	)
}

// SetupAndServeHTTP sets up the HTTP server to serve endpoints registered on
// ApiMux under /api/ and adds a prometheus handler under /metrics.
// nolint:gocyclo
func (b *BaseDendrite) SetupAndServeHTTP(
	internalHTTPAddr, externalHTTPAddr config.HTTPAddress,
	certFile, keyFile *string,
) {
	internalAddr, _ := internalHTTPAddr.Address()
	externalAddr, _ := externalHTTPAddr.Address()

	internalRouter := mux.NewRouter()
	externalRouter := internalRouter

	internalServ := &http.Server{
		Addr:         string(internalAddr),
		WriteTimeout: HTTPServerTimeout,
		Handler:      internalRouter,
	}
	externalServ := internalServ

	if externalAddr != NoExternalListener && externalAddr != internalAddr {
		externalRouter = mux.NewRouter()
		externalServ = &http.Server{
			Addr:         string(externalAddr),
			WriteTimeout: HTTPServerTimeout,
			Handler:      externalRouter,
		}
	}

	internalRouter.PathPrefix(httputil.InternalPathPrefix).Handler(b.InternalAPIMux)
	if b.Cfg.Global.Metrics.Enabled {
		internalRouter.Handle("/metrics", httputil.WrapHandlerInBasicAuth(promhttp.Handler(), b.Cfg.Global.Metrics.BasicAuth))
	}

	externalRouter.PathPrefix(httputil.PublicClientPathPrefix).Handler(b.PublicClientAPIMux)
	externalRouter.PathPrefix(httputil.PublicKeyPathPrefix).Handler(b.PublicKeyAPIMux)
	externalRouter.PathPrefix(httputil.PublicFederationPathPrefix).Handler(b.PublicFederationAPIMux)
	externalRouter.PathPrefix(httputil.PublicMediaPathPrefix).Handler(b.PublicMediaAPIMux)

	go func() {
		logrus.Infof("Starting %s listener on %s", b.componentName, internalServ.Addr)
		if certFile != nil && keyFile != nil {
			if err := internalServ.ListenAndServeTLS(*certFile, *keyFile); err != nil {
				logrus.WithError(err).Fatal("failed to serve HTTPS")
			}
		} else {
			if err := internalServ.ListenAndServe(); err != nil {
				logrus.WithError(err).Fatal("failed to serve HTTP")
			}
		}
		logrus.Infof("Stopped %s listener on %s", b.componentName, internalServ.Addr)
	}()

	if externalAddr != NoExternalListener && internalAddr != externalAddr {
		go func() {
			logrus.Infof("Starting %s listener on %s", b.componentName, externalServ.Addr)
			if certFile != nil && keyFile != nil {
				if err := externalServ.ListenAndServeTLS(*certFile, *keyFile); err != nil {
					logrus.WithError(err).Fatal("failed to serve HTTPS")
				}
			} else {
				if err := externalServ.ListenAndServe(); err != nil {
					logrus.WithError(err).Fatal("failed to serve HTTP")
				}
			}
			logrus.Infof("Stopped %s listener on %s", b.componentName, externalServ.Addr)
		}()
	}

	select {}
}

// setupKafka creates kafka consumer/producer pair from the config.
func setupKafka(cfg *config.Dendrite) (sarama.Consumer, sarama.SyncProducer) {
	consumer, err := sarama.NewConsumer(cfg.Global.Kafka.Addresses, nil)
	if err != nil {
		logrus.WithError(err).Panic("failed to start kafka consumer")
	}

	producer, err := sarama.NewSyncProducer(cfg.Global.Kafka.Addresses, nil)
	if err != nil {
		logrus.WithError(err).Panic("failed to setup kafka producers")
	}

	return consumer, producer
}

// setupNaffka creates kafka consumer/producer pair from the config.
func setupNaffka(cfg *config.Dendrite) (sarama.Consumer, sarama.SyncProducer) {
	var naffkaDB *naffka.DatabaseImpl

	db, err := sqlutil.Open(&cfg.Global.Kafka.Database)
	if err != nil {
		logrus.WithError(err).Panic("Failed to open naffka database")
	}

	switch {
	case cfg.Global.Kafka.Database.ConnectionString.IsSQLite():
		naffkaDB, err = naffka.NewSqliteDatabase(db)
		if err != nil {
			logrus.WithError(err).Panic("Failed to setup naffka database")
		}
	case cfg.Global.Kafka.Database.ConnectionString.IsPostgres():
		naffkaDB, err = naffka.NewPostgresqlDatabase(db)
		if err != nil {
			logrus.WithError(err).Panic("Failed to setup naffka database")
		}
	default:
		panic("unknown naffka database type")
	}

	if naffkaDB == nil {
		panic("naffka connection string not understood")
	}

	naff, err := naffka.New(naffkaDB)
	if err != nil {
		logrus.WithError(err).Panic("Failed to setup naffka")
	}

	return naff, naff
}
