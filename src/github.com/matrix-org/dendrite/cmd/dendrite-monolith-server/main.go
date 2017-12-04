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

package main

import (
	"context"
	"database/sql"
	"flag"
	"net/http"
	"os"

	"github.com/opentracing/opentracing-go"

	"github.com/gorilla/mux"
	"github.com/matrix-org/dendrite/clientapi/auth/storage/accounts"
	"github.com/matrix-org/dendrite/clientapi/auth/storage/devices"
	"github.com/matrix-org/dendrite/clientapi/producers"
	"github.com/matrix-org/dendrite/common"
	"github.com/matrix-org/dendrite/common/config"
	"github.com/matrix-org/dendrite/common/keydb"
	"github.com/matrix-org/gomatrixserverlib"
	"github.com/matrix-org/naffka"

	mediaapi_routing "github.com/matrix-org/dendrite/mediaapi/routing"
	mediaapi_storage "github.com/matrix-org/dendrite/mediaapi/storage"

	roomserver_alias "github.com/matrix-org/dendrite/roomserver/alias"
	roomserver_input "github.com/matrix-org/dendrite/roomserver/input"
	roomserver_query "github.com/matrix-org/dendrite/roomserver/query"
	roomserver_storage "github.com/matrix-org/dendrite/roomserver/storage"

	clientapi_consumers "github.com/matrix-org/dendrite/clientapi/consumers"
	clientapi_routing "github.com/matrix-org/dendrite/clientapi/routing"

	syncapi_consumers "github.com/matrix-org/dendrite/syncapi/consumers"
	syncapi_routing "github.com/matrix-org/dendrite/syncapi/routing"
	syncapi_storage "github.com/matrix-org/dendrite/syncapi/storage"
	syncapi_sync "github.com/matrix-org/dendrite/syncapi/sync"
	syncapi_types "github.com/matrix-org/dendrite/syncapi/types"

	federationapi_routing "github.com/matrix-org/dendrite/federationapi/routing"

	federationsender_consumers "github.com/matrix-org/dendrite/federationsender/consumers"
	"github.com/matrix-org/dendrite/federationsender/queue"
	federationsender_storage "github.com/matrix-org/dendrite/federationsender/storage"

	publicroomsapi_consumers "github.com/matrix-org/dendrite/publicroomsapi/consumers"
	publicroomsapi_routing "github.com/matrix-org/dendrite/publicroomsapi/routing"
	publicroomsapi_storage "github.com/matrix-org/dendrite/publicroomsapi/storage"

	log "github.com/sirupsen/logrus"
	sarama "gopkg.in/Shopify/sarama.v1"
)

var (
	logDir        = os.Getenv("LOG_DIR")
	configPath    = flag.String("config", "dendrite.yaml", "The path to the config file. For more information, see the config file in this repository.")
	httpBindAddr  = flag.String("http-bind-address", ":8008", "The HTTP listening port for the server")
	httpsBindAddr = flag.String("https-bind-address", ":8448", "The HTTPS listening port for the server")
	certFile      = flag.String("tls-cert", "", "The PEM formatted X509 certificate to use for TLS")
	keyFile       = flag.String("tls-key", "", "The PEM private key to use for TLS")
)

func main() {
	common.SetupLogging(logDir)

	flag.Parse()

	if *configPath == "" {
		log.Fatal("--config must be supplied")
	}
	cfg, err := config.LoadMonolithic(*configPath)
	if err != nil {
		log.Fatalf("Invalid config file: %s", err)
	}

	tracers := common.NewTracers(cfg)
	defer tracers.Close() // nolint: errcheck

	err = tracers.InitGlobalTracer("DendriteMonolith")
	if err != nil {
		log.WithError(err).Fatalf("Failed to start tracer")
	}

	m := newMonolith(cfg, tracers)
	m.setupDatabases()
	m.setupFederation()
	m.setupKafka()
	m.setupRoomServer()
	m.setupProducers()
	m.setupNotifiers()
	m.setupConsumers()
	m.setupAPIs()

	// Expose the matrix APIs directly rather than putting them under a /api path.
	go func() {
		log.Info("Listening on ", *httpBindAddr)
		log.Fatal(http.ListenAndServe(*httpBindAddr, common.WrapHandlerInCORS(m.api)))
	}()
	// Handle HTTPS if certificate and key are provided
	go func() {
		if *certFile != "" && *keyFile != "" {
			log.Info("Listening on ", *httpsBindAddr)
			log.Fatal(http.ListenAndServeTLS(*httpsBindAddr, *certFile, *keyFile, m.api))
		}
	}()

	// We want to block forever to let the HTTP and HTTPS handler serve the APIs
	select {}
}

// A monolith contains all the dendrite components.
// Some of the setup functions depend on previous setup functions, so they must
// be called in the same order as they are defined in the file.
type monolith struct {
	cfg *config.Dendrite
	api *mux.Router

	roomServerDB       *roomserver_storage.Database
	accountDB          *accounts.Database
	deviceDB           *devices.Database
	keyDB              *keydb.Database
	mediaAPIDB         *mediaapi_storage.Database
	syncAPIDB          *syncapi_storage.SyncServerDatabase
	federationSenderDB *federationsender_storage.Database
	publicRoomsAPIDB   *publicroomsapi_storage.PublicRoomsServerDatabase

	federation *gomatrixserverlib.FederationClient
	keyRing    gomatrixserverlib.KeyRing

	inputAPI *roomserver_input.InProcessRoomServerInput
	queryAPI *roomserver_query.InProcessRoomServerQueryAPI
	aliasAPI *roomserver_alias.RoomserverAliasAPI

	naffka        *naffka.Naffka
	kafkaProducer sarama.SyncProducer

	roomServerProducer *producers.RoomserverProducer
	userUpdateProducer *producers.UserUpdateProducer
	syncProducer       *producers.SyncAPIProducer

	syncAPINotifier *syncapi_sync.Notifier

	tracers              *common.Tracers
	clientAPITracer      opentracing.Tracer
	syncAPITracer        opentracing.Tracer
	mediaAPITracer       opentracing.Tracer
	federationAPITracer  opentracing.Tracer
	publicRoomsAPITracer opentracing.Tracer
	roomServerTracer     opentracing.Tracer
}

func newMonolith(cfg *config.Dendrite, tracers *common.Tracers) *monolith {
	return &monolith{
		cfg:                  cfg,
		api:                  mux.NewRouter(),
		tracers:              tracers,
		clientAPITracer:      tracers.SetupNewTracer("ClientAPI"),
		syncAPITracer:        tracers.SetupNewTracer("SyncAPI"),
		mediaAPITracer:       tracers.SetupNewTracer("MediaAPI"),
		federationAPITracer:  tracers.SetupNewTracer("FederationAPI"),
		publicRoomsAPITracer: tracers.SetupNewTracer("PublicRooms"),
		roomServerTracer:     tracers.SetupNewTracer("RoomServer"),
	}
}

func (m *monolith) setupDatabases() {
	var err error
	m.roomServerDB, err = roomserver_storage.Open(m.tracers, string(m.cfg.Database.RoomServer))
	if err != nil {
		panic(err)
	}
	m.accountDB, err = accounts.NewDatabase(m.tracers, string(m.cfg.Database.Account), m.cfg.Matrix.ServerName)
	if err != nil {
		log.Panicf("Failed to setup account database(%q): %s", m.cfg.Database.Account, err.Error())
	}
	m.deviceDB, err = devices.NewDatabase(m.tracers, string(m.cfg.Database.Device), m.cfg.Matrix.ServerName)
	if err != nil {
		log.Panicf("Failed to setup device database(%q): %s", m.cfg.Database.Device, err.Error())
	}
	m.keyDB, err = keydb.NewDatabase(m.tracers, string(m.cfg.Database.ServerKey))
	if err != nil {
		log.Panicf("Failed to setup key database(%q): %s", m.cfg.Database.ServerKey, err.Error())
	}
	m.mediaAPIDB, err = mediaapi_storage.Open(m.tracers, string(m.cfg.Database.MediaAPI))
	if err != nil {
		log.Panicf("Failed to setup sync api database(%q): %s", m.cfg.Database.MediaAPI, err.Error())
	}
	m.syncAPIDB, err = syncapi_storage.NewSyncServerDatabase(m.tracers, string(m.cfg.Database.SyncAPI))
	if err != nil {
		log.Panicf("Failed to setup sync api database(%q): %s", m.cfg.Database.SyncAPI, err.Error())
	}
	m.federationSenderDB, err = federationsender_storage.NewDatabase(m.tracers, string(m.cfg.Database.FederationSender))
	if err != nil {
		log.Panicf("startup: failed to create federation sender database with data source %s : %s", m.cfg.Database.FederationSender, err)
	}
	m.publicRoomsAPIDB, err = publicroomsapi_storage.NewPublicRoomsServerDatabase(m.tracers, string(m.cfg.Database.PublicRoomsAPI))
	if err != nil {
		log.Panicf("startup: failed to setup public rooms api database with data source %s : %s", m.cfg.Database.PublicRoomsAPI, err)
	}
}

func (m *monolith) setupFederation() {
	m.federation = gomatrixserverlib.NewFederationClient(
		m.cfg.Matrix.ServerName, m.cfg.Matrix.KeyID, m.cfg.Matrix.PrivateKey,
	)

	m.keyRing = keydb.CreateKeyRing(m.federation.Client, m.keyDB)
}

func (m *monolith) setupKafka() {
	if m.cfg.Kafka.UseNaffka {
		db, err := sql.Open("postgres", string(m.cfg.Database.Naffka))
		if err != nil {
			log.WithFields(log.Fields{
				log.ErrorKey: err,
			}).Panic("Failed to open naffka database")
		}

		naffkaDB, err := naffka.NewPostgresqlDatabase(db)
		if err != nil {
			log.WithFields(log.Fields{
				log.ErrorKey: err,
			}).Panic("Failed to setup naffka database")
		}

		naff, err := naffka.New(naffkaDB)
		if err != nil {
			log.WithFields(log.Fields{
				log.ErrorKey: err,
			}).Panic("Failed to setup naffka")
		}
		m.naffka = naff
		m.kafkaProducer = naff
	} else {
		var err error
		m.kafkaProducer, err = sarama.NewSyncProducer(m.cfg.Kafka.Addresses, nil)
		if err != nil {
			log.WithFields(log.Fields{
				log.ErrorKey: err,
				"addresses":  m.cfg.Kafka.Addresses,
			}).Panic("Failed to setup kafka producers")
		}
	}
}

func (m *monolith) kafkaConsumer() sarama.Consumer {
	if m.cfg.Kafka.UseNaffka {
		return m.naffka
	}
	consumer, err := sarama.NewConsumer(m.cfg.Kafka.Addresses, nil)
	if err != nil {
		log.WithFields(log.Fields{
			log.ErrorKey: err,
			"addresses":  m.cfg.Kafka.Addresses,
		}).Panic("Failed to setup kafka consumers")
	}
	return consumer
}

func (m *monolith) setupRoomServer() {
	m.inputAPI = roomserver_input.NewInProcessRoomServerInput(
		roomserver_input.RoomserverInputAPI{
			DB:                   m.roomServerDB,
			Producer:             m.kafkaProducer,
			OutputRoomEventTopic: string(m.cfg.Kafka.Topics.OutputRoomEvent),
		},
		m.roomServerTracer,
	)

	q := roomserver_query.NewInProcessRoomServerQueryAPI(
		roomserver_query.RoomserverQueryAPI{
			DB: m.roomServerDB,
		},
		m.roomServerTracer,
	)
	m.queryAPI = &q

	m.aliasAPI = &roomserver_alias.RoomserverAliasAPI{
		DB:       m.roomServerDB,
		Cfg:      m.cfg,
		InputAPI: m.inputAPI,
		QueryAPI: m.queryAPI,
	}
}

func (m *monolith) setupProducers() {
	m.roomServerProducer = producers.NewRoomserverProducer(m.inputAPI)
	m.userUpdateProducer = &producers.UserUpdateProducer{
		Producer: m.kafkaProducer,
		Topic:    string(m.cfg.Kafka.Topics.UserUpdates),
	}
	m.syncProducer = &producers.SyncAPIProducer{
		Producer: m.kafkaProducer,
		Topic:    string(m.cfg.Kafka.Topics.OutputClientData),
	}
}

func (m *monolith) setupNotifiers() {
	pos, err := m.syncAPIDB.SyncStreamPosition(context.Background())
	if err != nil {
		log.Panicf("startup: failed to get latest sync stream position : %s", err)
	}

	m.syncAPINotifier = syncapi_sync.NewNotifier(syncapi_types.StreamPosition(pos))
	if err = m.syncAPINotifier.Load(context.Background(), m.syncAPIDB); err != nil {
		log.Panicf("startup: failed to set up notifier: %s", err)
	}
}

func (m *monolith) setupConsumers() {
	var err error

	clientAPIConsumer := clientapi_consumers.NewOutputRoomEventConsumer(
		m.cfg, m.kafkaConsumer(), m.accountDB, m.queryAPI,
		m.clientAPITracer,
	)
	if err = clientAPIConsumer.Start(); err != nil {
		log.Panicf("startup: failed to start room server consumer: %s", err)
	}

	syncAPIRoomConsumer := syncapi_consumers.NewOutputRoomEventConsumer(
		m.cfg, m.kafkaConsumer(), m.syncAPINotifier, m.syncAPIDB, m.queryAPI,
		m.syncAPITracer,
	)
	if err = syncAPIRoomConsumer.Start(); err != nil {
		log.Panicf("startup: failed to start room server consumer: %s", err)
	}

	syncAPIClientConsumer := syncapi_consumers.NewOutputClientDataConsumer(
		m.cfg, m.kafkaConsumer(), m.syncAPINotifier, m.syncAPIDB,
	)
	if err = syncAPIClientConsumer.Start(); err != nil {
		log.Panicf("startup: failed to start client API server consumer: %s", err)
	}

	publicRoomsAPIConsumer := publicroomsapi_consumers.NewOutputRoomEventConsumer(
		m.cfg, m.kafkaConsumer(), m.publicRoomsAPIDB, m.queryAPI, m.publicRoomsAPITracer,
	)
	if err = publicRoomsAPIConsumer.Start(); err != nil {
		log.Panicf("startup: failed to start room server consumer: %s", err)
	}

	federationSenderQueues := queue.NewOutgoingQueues(m.cfg.Matrix.ServerName, m.federation)

	federationSenderRoomConsumer := federationsender_consumers.NewOutputRoomEventConsumer(
		m.cfg, m.kafkaConsumer(), federationSenderQueues, m.federationSenderDB, m.queryAPI,
		m.federationAPITracer,
	)
	if err = federationSenderRoomConsumer.Start(); err != nil {
		log.WithError(err).Panicf("startup: failed to start room server consumer")
	}
}

func (m *monolith) setupAPIs() {
	clientapi_routing.Setup(
		m.api, *m.cfg, m.roomServerProducer,
		m.queryAPI, m.aliasAPI, m.accountDB, m.deviceDB, m.federation, m.keyRing,
		m.userUpdateProducer, m.syncProducer,
		m.clientAPITracer,
	)

	mediaapi_routing.Setup(
		m.api, m.cfg, m.mediaAPIDB, m.deviceDB, &m.federation.Client,
		m.mediaAPITracer,
	)

	syncapi_routing.Setup(m.api, syncapi_sync.NewRequestPool(
		m.syncAPIDB, m.syncAPINotifier, m.accountDB,
	), m.syncAPIDB, m.deviceDB, m.syncAPITracer)

	federationapi_routing.Setup(
		m.api, *m.cfg, m.queryAPI, m.aliasAPI, m.roomServerProducer, m.keyRing, m.federation,
		m.accountDB, m.federationAPITracer,
	)

	publicroomsapi_routing.Setup(m.api, m.deviceDB, m.publicRoomsAPIDB, m.publicRoomsAPITracer)
}
