// Copyright 2017 New Vector Ltd
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
	"database/sql"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"

	"github.com/matrix-org/dendrite/internal/caching"
	"github.com/matrix-org/dendrite/internal/httpapis"
	"github.com/matrix-org/dendrite/internal/sqlutil"
	"github.com/matrix-org/gomatrixserverlib"
	"github.com/matrix-org/naffka"

	"github.com/matrix-org/dendrite/clientapi/auth/storage/accounts"
	"github.com/matrix-org/dendrite/clientapi/auth/storage/devices"
	"github.com/matrix-org/dendrite/internal"

	"github.com/Shopify/sarama"
	"github.com/gorilla/mux"

	appserviceAPI "github.com/matrix-org/dendrite/appservice/api"
	asinthttp "github.com/matrix-org/dendrite/appservice/inthttp"
	eduServerAPI "github.com/matrix-org/dendrite/eduserver/api"
	eduinthttp "github.com/matrix-org/dendrite/eduserver/inthttp"
	federationSenderAPI "github.com/matrix-org/dendrite/federationsender/api"
	fsinthttp "github.com/matrix-org/dendrite/federationsender/inthttp"
	"github.com/matrix-org/dendrite/internal/config"
	roomserverAPI "github.com/matrix-org/dendrite/roomserver/api"
	rsinthttp "github.com/matrix-org/dendrite/roomserver/inthttp"
	serverKeyAPI "github.com/matrix-org/dendrite/serverkeyapi/api"
	skinthttp "github.com/matrix-org/dendrite/serverkeyapi/inthttp"
	"github.com/sirupsen/logrus"

	_ "net/http/pprof"
)

// Base is a base for creating new instances of dendrite. It parses
// command line flags and config, and exposes methods for creating various
// resources. All errors are handled by logging then exiting, so all methods
// should only be used during start up.
// Must be closed when shutting down.
type Base struct {
	componentName string
	tracerCloser  io.Closer

	// PublicAPIMux should be used to register new public matrix api endpoints
	PublicAPIMux   *mux.Router
	InternalAPIMux *mux.Router
	UseHTTPAPIs    bool
	httpClient     *http.Client
	Cfg            *config.Dendrite
	ImmutableCache caching.ImmutableCache
	KafkaConsumer  sarama.Consumer
	KafkaProducer  sarama.SyncProducer

	// internal APIs
	appserviceAPI    appserviceAPI.AppServiceQueryAPI
	roomserverAPI    roomserverAPI.RoomserverInternalAPI
	eduServer        eduServerAPI.EDUServerInputAPI
	federationSender federationSenderAPI.FederationSenderInternalAPI
	serverKeyAPI     serverKeyAPI.ServerKeyInternalAPI

	DeviceDB         devices.Database
	AccountDB        accounts.Database
	FederationClient *gomatrixserverlib.FederationClient
}

const HTTPServerTimeout = time.Minute * 5
const HTTPClientTimeout = time.Second * 30

// NewBase creates a new instance to be used by a component.
// The componentName is used for logging purposes, and should be a friendly name
// of the compontent running, e.g. "SyncAPI"
// If `useHTTPAPIs` is true, HTTP clients will be made for all internal APIs.
func NewBase(cfg *config.Dendrite, componentName string, useHTTPAPIs bool) *Base {
	internal.SetupStdLogging()
	internal.SetupHookLogging(cfg.Logging, componentName)
	internal.SetupPprof()

	closer, err := cfg.SetupTracing("Dendrite" + componentName)
	if err != nil {
		logrus.WithError(err).Panicf("failed to start opentracing")
	}

	var kafkaConsumer sarama.Consumer
	var kafkaProducer sarama.SyncProducer
	if cfg.Kafka.UseNaffka {
		kafkaConsumer, kafkaProducer = setupNaffka(cfg)
	} else {
		kafkaConsumer, kafkaProducer = setupKafka(cfg)
	}

	cache, err := caching.NewImmutableInMemoryLRUCache()
	if err != nil {
		logrus.WithError(err).Warnf("Failed to create cache")
	}

	client := http.Client{Timeout: HTTPClientTimeout}
	if cfg.Proxy != nil {
		client.Transport = &http.Transport{Proxy: http.ProxyURL(&url.URL{
			Scheme: cfg.Proxy.Protocol,
			Host:   fmt.Sprintf("%s:%d", cfg.Proxy.Host, cfg.Proxy.Port),
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
	httpmux := mux.NewRouter().SkipClean(true)

	b := &Base{
		componentName:  componentName,
		UseHTTPAPIs:    useHTTPAPIs,
		tracerCloser:   closer,
		Cfg:            cfg,
		ImmutableCache: cache,
		PublicAPIMux:   httpmux.PathPrefix(httpapis.PublicPathPrefix).Subrouter().UseEncodedPath(),
		InternalAPIMux: httpmux.PathPrefix(httpapis.InternalPathPrefix).Subrouter().UseEncodedPath(),
		httpClient:     &client,
		KafkaConsumer:  kafkaConsumer,
		KafkaProducer:  kafkaProducer,
	}

	if useHTTPAPIs {
		b.appserviceAPI = b.createAppserviceHTTPClient()
		b.roomserverAPI = b.createRoomserverHTTPClient()
		b.eduServer = b.createEDUServerClient()
		b.federationSender = b.createFederationSenderHTTPClient()
		b.serverKeyAPI = b.createServerKeyAPIClient()
	}

	b.DeviceDB = b.createDeviceDB()
	b.AccountDB = b.createAccountsDB()
	b.FederationClient = b.createFederationClient()

	return b
}

// Close implements io.Closer
func (b *Base) Close() error {
	return b.tracerCloser.Close()
}

// AppserviceAPI return the API or panics if one is not set.
func (b *Base) AppserviceAPI() appserviceAPI.AppServiceQueryAPI {
	if b.appserviceAPI == nil {
		logrus.Panic("AppserviceAPI is unset")
	}
	return b.appserviceAPI
}

// RoomserverAPI return the API or panics if one is not set.
func (b *Base) RoomserverAPI() roomserverAPI.RoomserverInternalAPI {
	if b.roomserverAPI == nil {
		logrus.Panic("RoomserverAPI is unset")
	}
	return b.roomserverAPI
}

// EDUServer return the API or panics if one is not set.
func (b *Base) EDUServer() eduServerAPI.EDUServerInputAPI {
	if b.eduServer == nil {
		logrus.Panic("EDUServer is unset")
	}
	return b.eduServer
}

// FederationSender return the API or panics if one is not set.
func (b *Base) FederationSender() federationSenderAPI.FederationSenderInternalAPI {
	if b.federationSender == nil {
		logrus.Panic("FederationSender is unset")
	}
	return b.federationSender
}

// ServerKeyAPI return the API or panics if one is not set.
func (b *Base) ServerKeyAPI() serverKeyAPI.ServerKeyInternalAPI {
	if b.serverKeyAPI == nil {
		logrus.Panic("ServerKeyAPI is unset")
	}
	return b.serverKeyAPI
}

func (b *Base) SetServerKeyAPI(a serverKeyAPI.ServerKeyInternalAPI) {
	b.serverKeyAPI = a
}
func (b *Base) SetFederationSender(a federationSenderAPI.FederationSenderInternalAPI) {
	b.federationSender = a
}
func (b *Base) SetEDUServer(a eduServerAPI.EDUServerInputAPI) {
	b.eduServer = a
}
func (b *Base) SetRoomserverAPI(a roomserverAPI.RoomserverInternalAPI) {
	b.roomserverAPI = a
}
func (b *Base) SetAppserviceAPI(a appserviceAPI.AppServiceQueryAPI) {
	b.appserviceAPI = a
}

// createAppserviceHTTPClient returns the AppServiceQueryAPI for hitting the appservice component over HTTP.
func (b *Base) createAppserviceHTTPClient() appserviceAPI.AppServiceQueryAPI {
	a, err := asinthttp.NewAppserviceClient(b.Cfg.AppServiceURL(), b.httpClient)
	if err != nil {
		logrus.WithError(err).Panic("CreateHTTPAppServiceAPIs failed")
	}
	return a
}

// createRoomserverHTTPClient returns RoomserverInternalAPI for hitting the roomserver over HTTP.
func (b *Base) createRoomserverHTTPClient() roomserverAPI.RoomserverInternalAPI {
	rsAPI, err := rsinthttp.NewRoomserverClient(b.Cfg.RoomServerURL(), b.httpClient, b.ImmutableCache)
	if err != nil {
		logrus.WithError(err).Panic("RoomserverHTTPClient failed", b.httpClient)
	}
	return rsAPI
}

// createEDUServerClient returns EDUServerInputAPI for hitting the EDU server over HTTP
func (b *Base) createEDUServerClient() eduServerAPI.EDUServerInputAPI {
	e, err := eduinthttp.NewEDUServerClient(b.Cfg.EDUServerURL(), b.httpClient)
	if err != nil {
		logrus.WithError(err).Panic("EDUServerClient failed", b.httpClient)
	}
	return e
}

// createFederationSenderHTTPClient returns FederationSenderInternalAPI for hitting
// the federation sender over HTTP
func (b *Base) createFederationSenderHTTPClient() federationSenderAPI.FederationSenderInternalAPI {
	f, err := fsinthttp.NewFederationSenderClient(b.Cfg.FederationSenderURL(), b.httpClient)
	if err != nil {
		logrus.WithError(err).Panic("FederationSenderHTTPClient failed", b.httpClient)
	}
	return f
}

// createServerKeyAPIClient returns ServerKeyInternalAPI for hitting the server key API over HTTP
func (b *Base) createServerKeyAPIClient() serverKeyAPI.ServerKeyInternalAPI {
	f, err := skinthttp.NewServerKeyClient(
		b.Cfg.ServerKeyAPIURL(),
		b.httpClient,
		b.ImmutableCache,
	)
	if err != nil {
		logrus.WithError(err).Panic("NewServerKeyInternalAPIHTTP failed", b.httpClient)
	}
	return f
}

// createDeviceDB creates a new instance of the device database. Should only be
// called once per component.
func (b *Base) createDeviceDB() devices.Database {
	if b.Cfg.Database.Device == "" {
		return nil
	}
	db, err := devices.NewDatabase(string(b.Cfg.Database.Device), b.Cfg.DbProperties(), b.Cfg.Matrix.ServerName)
	if err != nil {
		logrus.WithError(err).Panicf("failed to connect to devices db")
	}

	return db
}

// createAccountsDB creates a new instance of the accounts database. Should only
// be called once per component.
func (b *Base) createAccountsDB() accounts.Database {
	if b.Cfg.Database.Account == "" {
		return nil
	}
	db, err := accounts.NewDatabase(string(b.Cfg.Database.Account), b.Cfg.DbProperties(), b.Cfg.Matrix.ServerName)
	if err != nil {
		logrus.WithError(err).Panicf("failed to connect to accounts db")
	}

	return db
}

// createFederationClient creates a new federation client. Should only be called
// once per component.
func (b *Base) createFederationClient() *gomatrixserverlib.FederationClient {
	return gomatrixserverlib.NewFederationClient(
		b.Cfg.Matrix.ServerName, b.Cfg.Matrix.KeyID, b.Cfg.Matrix.PrivateKey,
	)
}

// SetupAndServeHTTP sets up the HTTP server to serve endpoints registered on
// ApiMux under /api/ and adds a prometheus handler under /metrics.
func (b *Base) SetupAndServeHTTP(bindaddr string, listenaddr string) {
	// If a separate bind address is defined, listen on that. Otherwise use
	// the listen address
	var addr string
	if bindaddr != "" {
		addr = bindaddr
	} else {
		addr = listenaddr
	}

	serv := http.Server{
		Addr:         addr,
		WriteTimeout: HTTPServerTimeout,
	}

	internal.SetupHTTPAPI(
		http.DefaultServeMux,
		b.PublicAPIMux,
		b.InternalAPIMux,
		b.Cfg,
		b.UseHTTPAPIs,
	)
	logrus.Infof("Starting %s server on %s", b.componentName, serv.Addr)

	err := serv.ListenAndServe()
	if err != nil {
		logrus.WithError(err).Fatal("failed to serve http")
	}

	logrus.Infof("Stopped %s server on %s", b.componentName, serv.Addr)
}

// setupKafka creates kafka consumer/producer pair from the config.
func setupKafka(cfg *config.Dendrite) (sarama.Consumer, sarama.SyncProducer) {
	consumer, err := sarama.NewConsumer(cfg.Kafka.Addresses, nil)
	if err != nil {
		logrus.WithError(err).Panic("failed to start kafka consumer")
	}

	producer, err := sarama.NewSyncProducer(cfg.Kafka.Addresses, nil)
	if err != nil {
		logrus.WithError(err).Panic("failed to setup kafka producers")
	}

	return consumer, producer
}

// setupNaffka creates kafka consumer/producer pair from the config.
func setupNaffka(cfg *config.Dendrite) (sarama.Consumer, sarama.SyncProducer) {
	var err error
	var db *sql.DB
	var naffkaDB *naffka.DatabaseImpl

	uri, err := url.Parse(string(cfg.Database.Naffka))
	if err != nil || uri.Scheme == "file" {
		var cs string
		cs, err = sqlutil.ParseFileURI(string(cfg.Database.Naffka))
		if err != nil {
			logrus.WithError(err).Panic("Failed to parse naffka database file URI")
		}
		db, err = sqlutil.Open(internal.SQLiteDriverName(), cs, nil)
		if err != nil {
			logrus.WithError(err).Panic("Failed to open naffka database")
		}

		naffkaDB, err = naffka.NewSqliteDatabase(db)
		if err != nil {
			logrus.WithError(err).Panic("Failed to setup naffka database")
		}
	} else {
		db, err = sqlutil.Open("postgres", string(cfg.Database.Naffka), nil)
		if err != nil {
			logrus.WithError(err).Panic("Failed to open naffka database")
		}

		naffkaDB, err = naffka.NewPostgresqlDatabase(db)
		if err != nil {
			logrus.WithError(err).Panic("Failed to setup naffka database")
		}
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
