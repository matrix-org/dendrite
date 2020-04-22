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

	"github.com/matrix-org/gomatrixserverlib"
)

type Database interface {
	StoreEvent(ctx context.Context, appServiceID string, event *gomatrixserverlib.HeaderedEvent) error
	GetEventsWithAppServiceID(ctx context.Context, appServiceID string, limit int) (int, int, []gomatrixserverlib.HeaderedEvent, bool, error)
	CountEventsWithAppServiceID(ctx context.Context, appServiceID string) (int, error)
	UpdateTxnIDForEvents(ctx context.Context, appserviceID string, maxID, txnID int) error
	RemoveEventsBeforeAndIncludingID(ctx context.Context, appserviceID string, eventTableID int) error
	GetLatestTxnID(ctx context.Context) (int, error)
}
