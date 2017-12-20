// Copyright 2017 Vector Creations Ltd
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
	"flag"
	"io"
	"net/http"
	"os"

	"github.com/matrix-org/dendrite/common/keydb"
	"github.com/matrix-org/gomatrixserverlib"
	"github.com/matrix-org/naffka"

	"github.com/matrix-org/dendrite/clientapi/auth/storage/accounts"
	"github.com/matrix-org/dendrite/clientapi/auth/storage/devices"
	"github.com/matrix-org/dendrite/common"

	roomserver_alias "github.com/matrix-org/dendrite/roomserver/alias"
	roomserver_input "github.com/matrix-org/dendrite/roomserver/input"
	roomserver_query "github.com/matrix-org/dendrite/roomserver/query"
	roomserver_storage "github.com/matrix-org/dendrite/roomserver/storage"

	"github.com/gorilla/mux"
	sarama "gopkg.in/Shopify/sarama.v1"

	"github.com/matrix-org/dendrite/common/config"
	"github.com/matrix-org/dendrite/roomserver/api"
	"github.com/sirupsen/logrus"
)

var configPath = flag.String("config", "dendrite.yaml", "The path to the config file. For more information, see the config file in this repository.")

// BaseDendrite is a base for creating new instances of dendrite. It parses
// command line flags and config, and exposes methods for creating various
// resources. All errors are handled by logging then exiting, so all methods
// should only be used during start up.
// Must be closed when shutting down.
type BaseDendrite struct {
	componentName string
	tracerCloser  io.Closer
	queryAPI      api.RoomserverQueryAPI
	inputAPI      api.RoomserverInputAPI
	aliasAPI      api.RoomserverAliasAPI
	monolith      bool

	// APIMux should be used to register new public matrix api endpoints
	APIMux        *mux.Router
	Cfg           *config.Dendrite
	KafkaConsumer sarama.Consumer
	KafkaProducer sarama.SyncProducer
}

// NewBaseDendrite creates a new instance to be used by a component. If running
// as a monolith then `NewBaseDendriteMonolith` should be used.
// The componentName is used for logging purposes, and should be a friendly name
// of the compontent running, e.g. "SyncAPI"
func NewBaseDendrite(componentName string) *BaseDendrite {
	base := newBaseDendrite(componentName, false)

	// We're not a monolith so we can only use the HTTP versions
	base.useHTTPRoomserverAPIs()

	return base
}

// NewBaseDendriteMonolith is the same NewBaseDendrite, but indicates that all
// components will be in the same process. Allows using naffka and in-process
// roomserver APIs.
//
// It also connects to the room server databsae so that the monolith can use
// in-process versions of QueryAPI and co.
func NewBaseDendriteMonolith(componentName string) (*BaseDendrite, *roomserver_storage.Database) {
	base := newBaseDendrite(componentName, true)

	roomserverDB, err := roomserver_storage.Open(string(base.Cfg.Database.RoomServer))
	if err != nil {
		logrus.WithError(err).Panicf("failed to connect to room server db")
	}

	base.useInProcessRoomserverAPIs(roomserverDB)

	return base, roomserverDB
}

// newBaseDendrite does the bulk of the work of NewBaseDendrite*, except setting
// up the roomserver APIs, which must be done by the callers.
func newBaseDendrite(componentName string, monolith bool) *BaseDendrite {
	common.SetupLogging(os.Getenv("LOG_DIR"))

	flag.Parse()

	if *configPath == "" {
		logrus.Fatal("--config must be supplied")
	}

	var cfg *config.Dendrite
	var err error
	if monolith {
		cfg, err = config.LoadMonolithic(*configPath)
	} else {
		cfg, err = config.Load(*configPath)
	}
	if err != nil {
		logrus.Fatalf("Invalid config file: %s", err)
	}

	closer, err := cfg.SetupTracing("Dendrite" + componentName)
	if err != nil {
		logrus.WithError(err).Panicf("failed to start opentracing")
	}

	kafkaConsumer, kafkaProducer := setupKafka(cfg)

	return &BaseDendrite{
		componentName: componentName,
		tracerCloser:  closer,
		Cfg:           cfg,
		APIMux:        mux.NewRouter(),
		KafkaConsumer: kafkaConsumer,
		KafkaProducer: kafkaProducer,
		monolith:      monolith,
	}
}

// Close implements io.Closer
func (b *BaseDendrite) Close() error {
	return b.tracerCloser.Close()
}

// useInProcessRoomserverAPIs sets up the AliasAPI, InputAPI and QueryAPI to hit
// the functions directly, rather than going through an RPC mechanism. Can only
// be used in a monolith set up.
func (b *BaseDendrite) useInProcessRoomserverAPIs(roomserverDB *roomserver_storage.Database) {
	if !b.monolith {
		logrus.Panic("Can only use in-process roomserver APIs if running as a monolith")
	}

	b.inputAPI = &roomserver_input.RoomserverInputAPI{
		DB:                   roomserverDB,
		Producer:             b.KafkaProducer,
		OutputRoomEventTopic: string(b.Cfg.Kafka.Topics.OutputRoomEvent),
	}

	b.queryAPI = &roomserver_query.RoomserverQueryAPI{
		DB: roomserverDB,
	}

	b.aliasAPI = &roomserver_alias.RoomserverAliasAPI{
		DB:       roomserverDB,
		Cfg:      b.Cfg,
		InputAPI: b.inputAPI,
		QueryAPI: b.queryAPI,
	}
}

// useHTTPRoomserverAPIs sets up the AliasAPI, InputAPI and QueryAPI to hit the
// roomserver over HTTP.
func (b *BaseDendrite) useHTTPRoomserverAPIs() {
	b.queryAPI = api.NewRoomserverQueryAPIHTTP(b.Cfg.RoomServerURL(), nil)
	b.inputAPI = api.NewRoomserverInputAPIHTTP(b.Cfg.RoomServerURL(), nil)
	b.aliasAPI = api.NewRoomserverAliasAPIHTTP(b.Cfg.RoomServerURL(), nil)
}

// QueryAPI gets an implementation of RoomserverQueryAPI
func (b *BaseDendrite) QueryAPI() api.RoomserverQueryAPI {
	if b.queryAPI == nil {
		logrus.Panic("RoomserverAPIs not created")
	}

	return b.queryAPI
}

// AliasAPI gets an implementation of RoomserverAliasAPI
func (b *BaseDendrite) AliasAPI() api.RoomserverAliasAPI {
	if b.aliasAPI == nil {
		logrus.Panic("RoomserverAPIs not created")
	}

	return b.aliasAPI
}

// InputAPI gets an implementation of RoomserverInputAPI
func (b *BaseDendrite) InputAPI() api.RoomserverInputAPI {
	if b.inputAPI == nil {
		logrus.Panic("RoomserverAPIs not created")
	}

	return b.inputAPI
}

// CreateDeviceDB creates a new instance of the device database. Should only be
// called once per component.
func (b *BaseDendrite) CreateDeviceDB() *devices.Database {
	db, err := devices.NewDatabase(string(b.Cfg.Database.Device), b.Cfg.Matrix.ServerName)
	if err != nil {
		logrus.WithError(err).Panicf("failed to connect to devices db")
	}

	return db
}

// CreateAccountsDB creates a new instance of the accounts database. Should only
// be called once per component.
func (b *BaseDendrite) CreateAccountsDB() *accounts.Database {
	db, err := accounts.NewDatabase(string(b.Cfg.Database.Account), b.Cfg.Matrix.ServerName)
	if err != nil {
		logrus.WithError(err).Panicf("failed to connect to accounts db")
	}

	return db
}

// CreateKeyDB creates a new instance of the key database. Should only be called
// once per component.
func (b *BaseDendrite) CreateKeyDB() *keydb.Database {
	db, err := keydb.NewDatabase(string(b.Cfg.Database.ServerKey))
	if err != nil {
		logrus.WithError(err).Panicf("failed to connect to keys db")
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
func (b *BaseDendrite) SetupAndServeHTTP(addr string) {
	common.SetupHTTPAPI(http.DefaultServeMux, common.WrapHandlerInCORS(b.APIMux))

	logrus.Infof("Starting %s server on %s", b.componentName, addr)

	err := http.ListenAndServe(addr, nil)

	if err != nil {
		logrus.WithError(err).Fatal("failed to serve http")
	}

	logrus.Infof("Stopped %s server on %s", b.componentName, addr)
}

// setupKafka creates kafka consumer/producer pair from the config. Checks if
// should use naffka.
func setupKafka(cfg *config.Dendrite) (sarama.Consumer, sarama.SyncProducer) {
	if cfg.Kafka.UseNaffka {
		db, err := sql.Open("postgres", string(cfg.Database.Naffka))
		if err != nil {
			logrus.WithError(err).Panic("Failed to open naffka database")
		}

		naffkaDB, err := naffka.NewPostgresqlDatabase(db)
		if err != nil {
			logrus.WithError(err).Panic("Failed to setup naffka database")
		}

		naff, err := naffka.New(naffkaDB)
		if err != nil {
			logrus.WithError(err).Panic("Failed to setup naffka")
		}

		return naff, naff
	}

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
