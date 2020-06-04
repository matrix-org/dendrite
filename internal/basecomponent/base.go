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

package basecomponent

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
	eduServerAPI "github.com/matrix-org/dendrite/eduserver/api"
	federationSenderAPI "github.com/matrix-org/dendrite/federationsender/api"
	"github.com/matrix-org/dendrite/federationsender/inthttp"
	"github.com/matrix-org/dendrite/internal/config"
	roomserverAPI "github.com/matrix-org/dendrite/roomserver/api"
	serverKeyAPI "github.com/matrix-org/dendrite/serverkeyapi/api"
	"github.com/sirupsen/logrus"

	_ "net/http/pprof"
)

// BaseDendrite is a base for creating new instances of dendrite. It parses
// command line flags and config, and exposes methods for creating various
// resources. All errors are handled by logging then exiting, so all methods
// should only be used during start up.
// Must be closed when shutting down.
type BaseDendrite struct {
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
}

const HTTPServerTimeout = time.Minute * 5
const HTTPClientTimeout = time.Second * 30

// NewBaseDendrite creates a new instance to be used by a component.
// The componentName is used for logging purposes, and should be a friendly name
// of the compontent running, e.g. "SyncAPI"
func NewBaseDendrite(cfg *config.Dendrite, componentName string, useHTTPAPIs bool) *BaseDendrite {
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

	return &BaseDendrite{
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
}

// Close implements io.Closer
func (b *BaseDendrite) Close() error {
	return b.tracerCloser.Close()
}

// CreateHTTPAppServiceAPIs returns the QueryAPI for hitting the appservice
// component over HTTP.
func (b *BaseDendrite) CreateHTTPAppServiceAPIs() appserviceAPI.AppServiceQueryAPI {
	a, err := appserviceAPI.NewAppServiceQueryAPIHTTP(b.Cfg.AppServiceURL(), b.httpClient)
	if err != nil {
		logrus.WithError(err).Panic("CreateHTTPAppServiceAPIs failed")
	}
	return a
}

// CreateHTTPRoomserverAPIs returns the AliasAPI, InputAPI and QueryAPI for hitting
// the roomserver over HTTP.
func (b *BaseDendrite) CreateHTTPRoomserverAPIs() roomserverAPI.RoomserverInternalAPI {
	rsAPI, err := roomserverAPI.NewRoomserverInternalAPIHTTP(b.Cfg.RoomServerURL(), b.httpClient, b.ImmutableCache)
	if err != nil {
		logrus.WithError(err).Panic("NewRoomserverInternalAPIHTTP failed", b.httpClient)
	}
	return rsAPI
}

// CreateHTTPEDUServerAPIs returns eduInputAPI for hitting the EDU
// server over HTTP
func (b *BaseDendrite) CreateHTTPEDUServerAPIs() eduServerAPI.EDUServerInputAPI {
	e, err := eduServerAPI.NewEDUServerInputAPIHTTP(b.Cfg.EDUServerURL(), b.httpClient)
	if err != nil {
		logrus.WithError(err).Panic("NewEDUServerInputAPIHTTP failed", b.httpClient)
	}
	return e
}

// FederationSenderHTTPClient returns FederationSenderInternalAPI for hitting
// the federation sender over HTTP
func (b *BaseDendrite) FederationSenderHTTPClient() federationSenderAPI.FederationSenderInternalAPI {
	f, err := inthttp.NewFederationSenderClient(b.Cfg.FederationSenderURL(), b.httpClient)
	if err != nil {
		logrus.WithError(err).Panic("NewFederationSenderInternalAPIHTTP failed", b.httpClient)
	}
	return f
}

// CreateHTTPServerKeyAPIs returns ServerKeyInternalAPI for hitting the server key
// API over HTTP
func (b *BaseDendrite) CreateHTTPServerKeyAPIs() serverKeyAPI.ServerKeyInternalAPI {
	f, err := serverKeyAPI.NewServerKeyInternalAPIHTTP(
		b.Cfg.ServerKeyAPIURL(),
		b.httpClient,
		b.ImmutableCache,
	)
	if err != nil {
		logrus.WithError(err).Panic("NewServerKeyInternalAPIHTTP failed", b.httpClient)
	}
	return f
}

// CreateDeviceDB creates a new instance of the device database. Should only be
// called once per component.
func (b *BaseDendrite) CreateDeviceDB() devices.Database {
	db, err := devices.NewDatabase(string(b.Cfg.Database.Device), b.Cfg.DbProperties(), b.Cfg.Matrix.ServerName)
	if err != nil {
		logrus.WithError(err).Panicf("failed to connect to devices db")
	}

	return db
}

// CreateAccountsDB creates a new instance of the accounts database. Should only
// be called once per component.
func (b *BaseDendrite) CreateAccountsDB() accounts.Database {
	db, err := accounts.NewDatabase(string(b.Cfg.Database.Account), b.Cfg.DbProperties(), b.Cfg.Matrix.ServerName)
	if err != nil {
		logrus.WithError(err).Panicf("failed to connect to accounts db")
	}

	return db
}

// CreateFederationClient creates a new federation client. Should only be called
// once per component.
func (b *BaseDendrite) CreateFederationClient() *gomatrixserverlib.FederationClient {
	return gomatrixserverlib.NewFederationClient(
		b.Cfg.Matrix.ServerName, b.Cfg.Matrix.KeyID, b.Cfg.Matrix.PrivateKey,
	)
}

// SetupAndServeHTTP sets up the HTTP server to serve endpoints registered on
// ApiMux under /api/ and adds a prometheus handler under /metrics.
func (b *BaseDendrite) SetupAndServeHTTP(bindaddr string, listenaddr string) {
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
