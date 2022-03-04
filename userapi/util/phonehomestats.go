// Copyright 2022 The Matrix.org Foundation C.I.C.
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

package util

import (
	"bytes"
	"context"
	"encoding/json"
	"math"
	"net/http"
	"runtime"
	"syscall"
	"time"

	"github.com/matrix-org/dendrite/internal"
	"github.com/matrix-org/dendrite/internal/sqlutil"
	"github.com/matrix-org/dendrite/setup/config"
	"github.com/matrix-org/dendrite/userapi/storage"
	"github.com/matrix-org/gomatrixserverlib"
	"github.com/sirupsen/logrus"
)

type phoneHomeStats struct {
	prevData   timestampToRUUsage
	stats      map[string]interface{}
	serverName gomatrixserverlib.ServerName
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

func StartPhoneHomeCollector(startTime time.Time, cfg *config.Dendrite, userDB storage.Database) {

	p := phoneHomeStats{
		startTime:  startTime,
		serverName: cfg.Global.ServerName,
		cfg:        cfg,
		db:         userDB,
		isMonolith: cfg.IsMonolith,
		client: &http.Client{
			Timeout: time.Second * 30,
		},
	}

	// start initial run after 5min
	time.AfterFunc(time.Minute * 5, func() {
		p.collect()
	})

	// run every 3 hours
	ticker := time.NewTicker(time.Hour * 3)

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
	db, err := sqlutil.Open(&p.cfg.UserAPI.AccountDatabase)
	if err != nil {
		logrus.WithError(err).Error("unable to connecto to database")
		return
	}
	defer db.Close()

	dbVersion := "unknown"
	dbEngine := "unknown"
	switch {
	case p.cfg.UserAPI.AccountDatabase.ConnectionString.IsSQLite():
		dbEngine = "SQLite"
		row := db.QueryRow("select sqlite_version();")
		if err := row.Scan(&dbVersion); err != nil {
			logrus.WithError(err).Error("unable to query version")
			return
		}
	case p.cfg.UserAPI.AccountDatabase.ConnectionString.IsPostgres():
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
	// TODO: Find a solution to actually set these values
	p.stats["total_room_count"] = 0
	p.stats["daily_messages"] = 0
	p.stats["daily_sent_messages"] = 0
	p.stats["daily_e2ee_messages"] = 0
	p.stats["daily_sent_e2ee_messages"] = 0

	count, err := p.db.AllUsers(ctx)
	if err != nil {
		logrus.WithError(err).Error("unable to query AllUsers")
		return
	}
	p.stats["total_users"] = count

	count, err = p.db.NonBridgedUsers(ctx)
	if err != nil {
		logrus.WithError(err).Error("unable to query NonBridgedUsers")
		return
	}
	p.stats["total_nonbridged_users"] = count

	count, err = p.db.DailyUsers(ctx)
	if err != nil {
		logrus.WithError(err).Error("unable to query DailyUsers")
		return
	}
	p.stats["daily_active_users"] = count

	count, err = p.db.MonthlyUsers(ctx)
	if err != nil {
		logrus.WithError(err).Error("unable to query MonthlyUsers")
		return
	}
	p.stats["monthly_active_users"] = count

	res, err := p.db.RegisteredUserByType(ctx)
	if err != nil {
		logrus.WithError(err).Error("unable to query RegisteredUserByType")
		return
	}
	for t, c := range res {
		p.stats["daily_user_type_"+t] = c
	}

	res, err = p.db.R30Users(ctx)
	if err != nil {
		logrus.WithError(err).Error("unable to query R30Users")
		return
	}
	for t, c := range res {
		p.stats["r30_users_"+t] = c
	}
	res, err = p.db.R30UsersV2(ctx)
	if err != nil {
		logrus.WithError(err).Error("unable to query R30UsersV2")
		return
	}
	for t, c := range res {
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
		logrus.WithError(err).Warn("unable to send phone home stats")
		return
	}
}