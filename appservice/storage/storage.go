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

package storage

import (
	"context"
	"net/url"

	"github.com/matrix-org/dendrite/appservice/storage/postgres"
	"github.com/matrix-org/dendrite/appservice/storage/sqlite3"
	"github.com/matrix-org/gomatrixserverlib"
)

type Database interface {
	StoreEvent(ctx context.Context, appServiceID string, event *gomatrixserverlib.Event) error
	GetEventsWithAppServiceID(ctx context.Context, appServiceID string, limit int) (int, int, []gomatrixserverlib.Event, bool, error)
	CountEventsWithAppServiceID(ctx context.Context, appServiceID string) (int, error)
	UpdateTxnIDForEvents(ctx context.Context, appserviceID string, maxID, txnID int) error
	RemoveEventsBeforeAndIncludingID(ctx context.Context, appserviceID string, eventTableID int) error
	GetLatestTxnID(ctx context.Context) (int, error)
}

func NewDatabase(dataSourceName string) (Database, error) {
	uri, err := url.Parse(dataSourceName)
	if err != nil {
		return postgres.NewDatabase(dataSourceName)
	}
	switch uri.Scheme {
	case "postgres":
		return postgres.NewDatabase(dataSourceName)
	case "file":
		return sqlite3.NewDatabase(dataSourceName)
	default:
		return postgres.NewDatabase(dataSourceName)
	}
}
