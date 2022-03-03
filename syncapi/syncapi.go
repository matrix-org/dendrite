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

package syncapi

import (
	"bytes"
	"context"
	"encoding/json"
	"math"
	"net/http"
	"runtime"
	"syscall"
	"time"

	"github.com/gorilla/mux"
	"github.com/matrix-org/dendrite/internal"
	"github.com/matrix-org/dendrite/internal/sqlutil"
	"github.com/sirupsen/logrus"

	"github.com/matrix-org/dendrite/eduserver/cache"
	keyapi "github.com/matrix-org/dendrite/keyserver/api"
	"github.com/matrix-org/dendrite/roomserver/api"
	"github.com/matrix-org/dendrite/setup/config"
	"github.com/matrix-org/dendrite/setup/jetstream"
	"github.com/matrix-org/dendrite/setup/process"
	userapi "github.com/matrix-org/dendrite/userapi/api"
	"github.com/matrix-org/gomatrixserverlib"

	"github.com/matrix-org/dendrite/syncapi/consumers"
	"github.com/matrix-org/dendrite/syncapi/notifier"
	"github.com/matrix-org/dendrite/syncapi/routing"
	"github.com/matrix-org/dendrite/syncapi/storage"
	"github.com/matrix-org/dendrite/syncapi/streams"
	"github.com/matrix-org/dendrite/syncapi/sync"
)

// AddPublicRoutes sets up and registers HTTP handlers for the SyncAPI
// component.
func AddPublicRoutes(
	process *process.ProcessContext,
	router *mux.Router,
	userAPI userapi.UserInternalAPI,
	rsAPI api.RoomserverInternalAPI,
	keyAPI keyapi.KeyInternalAPI,
	federation *gomatrixserverlib.FederationClient,
	baseCfg *config.Dendrite,
	isMonolith bool,
) {
	cfg := &baseCfg.SyncAPI
	startTime := time.Now()
	js := jetstream.Prepare(&cfg.Matrix.JetStream)

	syncDB, err := storage.NewSyncServerDatasource(&cfg.Database, cfg.Matrix.ServerName)
	if err != nil {
		logrus.WithError(err).Panicf("failed to connect to sync db")
	}

	eduCache := cache.New()
	streams := streams.NewSyncStreamProviders(syncDB, userAPI, rsAPI, keyAPI, eduCache)
	notifier := notifier.NewNotifier(streams.Latest(context.Background()))
	if err = notifier.Load(context.Background(), syncDB); err != nil {
		logrus.WithError(err).Panicf("failed to load notifier ")
	}

	requestPool := sync.NewRequestPool(syncDB, cfg, userAPI, keyAPI, rsAPI, streams, notifier)

	keyChangeConsumer := consumers.NewOutputKeyChangeEventConsumer(
		process, cfg, cfg.Matrix.JetStream.TopicFor(jetstream.OutputKeyChangeEvent),
		js, keyAPI, rsAPI, syncDB, notifier,
		streams.DeviceListStreamProvider,
	)
	if err = keyChangeConsumer.Start(); err != nil {
		logrus.WithError(err).Panicf("failed to start key change consumer")
	}

	roomConsumer := consumers.NewOutputRoomEventConsumer(
		process, cfg, js, syncDB, notifier, streams.PDUStreamProvider,
		streams.InviteStreamProvider, rsAPI,
	)
	if err = roomConsumer.Start(); err != nil {
		logrus.WithError(err).Panicf("failed to start room server consumer")
	}

	clientConsumer := consumers.NewOutputClientDataConsumer(
		process, cfg, js, syncDB, notifier, streams.AccountDataStreamProvider,
	)
	if err = clientConsumer.Start(); err != nil {
		logrus.WithError(err).Panicf("failed to start client data consumer")
	}

	typingConsumer := consumers.NewOutputTypingEventConsumer(
		process, cfg, js, syncDB, eduCache, notifier, streams.TypingStreamProvider,
	)
	if err = typingConsumer.Start(); err != nil {
		logrus.WithError(err).Panicf("failed to start typing consumer")
	}

	sendToDeviceConsumer := consumers.NewOutputSendToDeviceEventConsumer(
		process, cfg, js, syncDB, notifier, streams.SendToDeviceStreamProvider,
	)
	if err = sendToDeviceConsumer.Start(); err != nil {
		logrus.WithError(err).Panicf("failed to start send-to-device consumer")
	}

	receiptConsumer := consumers.NewOutputReceiptEventConsumer(
		process, cfg, js, syncDB, notifier, streams.ReceiptStreamProvider,
	)
	if err = receiptConsumer.Start(); err != nil {
		logrus.WithError(err).Panicf("failed to start receipts consumer")
	}

	if baseCfg.Global.ReportStats {
		go startPhoneHomeCollector(startTime, baseCfg, syncDB, userAPI, isMonolith)
	}

	routing.Setup(router, requestPool, syncDB, userAPI, federation, rsAPI, cfg)
}

type phoneHomeStats struct {
	prevData   timestampToRUUsage
	stats      map[string]interface{}
	serverName gomatrixserverlib.ServerName
	userAPI    userapi.UserInternalAPI
	startTime  time.Time
	cfg        *config.Dendrite
	db         storage.Database
	isMonolith bool
	client     *http.Client
}

type timestampToRUUsage struct {
	timestamp int64
	usage     syscall.Rusage
}

func startPhoneHomeCollector(startTime time.Time, cfg *config.Dendrite, syncDB storage.Database, userAPI userapi.UserInternalAPI, isMonolith bool) {

	p := phoneHomeStats{
		startTime:  startTime,
		serverName: cfg.Global.ServerName,
		cfg:        cfg,
		db:         syncDB,
		userAPI:    userAPI,
		isMonolith: isMonolith,
		client: &http.Client{
			Timeout: time.Second * 30,
		},
	}

	// start initial run after 5min
	time.AfterFunc(time.Second*1, func() {
		p.collect()
	})

	// run every 3 hours
	ticker := time.NewTicker(time.Second * 10)

	for {
		select {
		case <-ticker.C:
			p.collect()
		}
	}
}

func (p *phoneHomeStats) collect() {
	p.stats = make(map[string]interface{})
	// general information
	p.stats["homeserver"] = p.serverName
	p.stats["monolith"] = p.isMonolith
	p.stats["version"] = internal.VersionString()
	p.stats["timestamp"] = time.Now().Unix()
	p.stats["go_version"] = runtime.Version()
	p.stats["go_arch"] = runtime.GOARCH
	p.stats["go_os"] = runtime.GOOS
	p.stats["num_cpu"] = runtime.NumCPU()
	p.stats["num_go_routine"] = runtime.NumGoroutine()
	p.stats["uptime_seconds"] = math.Floor(time.Now().Sub(p.startTime).Seconds())

	ctx, cancel := context.WithTimeout(context.TODO(), time.Minute*1)
	defer cancel()

	// cpu and memory usage information
	err := getMemoryStats(p)
	if err != nil {
		logrus.WithError(err).Error("unable to get memory/cpu stats")
		return
	}

	// configuration information
	p.stats["federation_disabled"] = p.cfg.Global.DisableFederation
	p.stats["nats_embedded"] = true
	p.stats["nats_in_memory"] = p.cfg.Global.JetStream.InMemory
	if len(p.cfg.Global.JetStream.Addresses) > 0 {
		p.stats["nats_embedded"] = false
		p.stats["nats_in_memory"] = false // probably
	}
	if len(p.cfg.Logging) > 0 {
		p.stats["log_level"] = p.cfg.Logging[0].Level
	} else {
		p.stats["log_level"] = "info"
	}

	// database configuration
	db, err := sqlutil.Open(&p.cfg.SyncAPI.Database)
	if err != nil {
		logrus.WithError(err).Error("unable to database")
		return
	}
	defer db.Close()

	dbVersion := "unknown"
	dbEngine := "unknown"
	switch {
	case p.cfg.SyncAPI.Database.ConnectionString.IsSQLite():
		dbEngine = "SQLite"
		row := db.QueryRow("select sqlite_version();")
		if err := row.Scan(&dbVersion); err != nil {
			logrus.WithError(err).Error("unable to query version")
			return
		}
	case p.cfg.SyncAPI.Database.ConnectionString.IsPostgres():
		dbEngine = "Postgres"
		row := db.QueryRow("SHOW server_version;")
		if err := row.Scan(&dbVersion); err != nil {
			logrus.WithError(err).Error("unable to query version")
			return
		}
	}
	p.stats["database_engine"] = dbEngine
	p.stats["database_server_version"] = dbVersion

	// message and room stats
	rooms, err := p.db.TotalRooms(ctx)
	if err != nil {
		logrus.WithError(err).Error("unable to query TotalRooms")
	}
	p.stats["total_room_count"] = rooms
	messages, err := p.db.DailyMessages(ctx, 0)
	if err != nil {
		logrus.WithError(err).Error("unable to query DailyMessages")
		return
	}
	p.stats["daily_messages"] = messages
	messages, err = p.db.DailySentMessages(ctx, 0)
	if err != nil {
		logrus.WithError(err).Error("unable to query DailySentMessages")
		return
	}
	p.stats["daily_sent_messages"] = messages

	messages, err = p.db.DailyE2EEMessages(ctx, 0)
	if err != nil {
		logrus.WithError(err).Error("unable to query DailyE2EEMessages")
		return
	}
	p.stats["daily_e2ee_messages"] = messages
	messages, err = p.db.DailySentE2EEMessages(ctx, 0)
	if err != nil {
		logrus.WithError(err).Error("unable to query DailySentE2EEMessages")
		return
	}
	p.stats["daily_sent_e2ee_messages"] = messages

	integerRes := &userapi.IntegerResponse{}
	if err = p.userAPI.AllUsers(ctx, integerRes); err != nil {
		logrus.WithError(err).Error("unable to query AllUsers")
		return
	}
	p.stats["total_users"] = integerRes.Count

	if err = p.userAPI.NonBridgedUsers(ctx, integerRes); err != nil {
		logrus.WithError(err).Error("unable to query NonBridgedUsers")
		return
	}
	p.stats["total_nonbridged_users"] = integerRes.Count

	if err = p.userAPI.DailyUsers(ctx, integerRes); err != nil {
		logrus.WithError(err).Error("unable to query DailyUsers")
		return
	}
	p.stats["daily_active_users"] = integerRes.Count

	if err = p.userAPI.MonthlyUsers(ctx, integerRes); err != nil {
		logrus.WithError(err).Error("unable to query MonthlyUsers")
		return
	}
	p.stats["monthly_active_users"] = integerRes.Count

	mapRes := &userapi.MapResponse{}
	if err = p.userAPI.RegisteredUserByType(ctx, mapRes); err != nil {
		logrus.WithError(err).Error("unable to query RegisteredUserByType")
		return
	}
	for t, c := range mapRes.Result {
		p.stats["daily_user_type_"+t] = c
	}

	if err = p.userAPI.R30Users(ctx, mapRes); err != nil {
		logrus.WithError(err).Error("unable to query R30Users")
		return
	}
	for t, c := range mapRes.Result {
		p.stats["r30_users_"+t] = c
	}

	if err = p.userAPI.R30UsersV2(ctx, mapRes); err != nil {
		logrus.WithError(err).Error("unable to query R30UsersV2")
		return
	}
	for t, c := range mapRes.Result {
		p.stats["r30v2_users_"+t] = c
	}

	output := bytes.Buffer{}
	if err = json.NewEncoder(&output).Encode(p.stats); err != nil {
		logrus.WithError(err).Error("unable to encode stats")
		return
	}

	logrus.Infof("Reporting stats to %s: %s", p.cfg.Global.ReportStatsEndpoint, output.String())

	request, err := http.NewRequest("POST", p.cfg.Global.ReportStatsEndpoint, &output)
	if err != nil {
		logrus.WithError(err).Error("unable to create phone home stats request")
		return
	}
	request.Header.Set("User-Agent", "Dendrite/"+internal.VersionString())

	_, err = p.client.Do(request)
	if err != nil {
		logrus.WithError(err).Error("unable to send phone home stats")
		return
	}
}