// Copyright 2021 Dan Peleg <dan@globekeeper.com>
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

package pushers

import (
	"context"

	"github.com/matrix-org/dendrite/userapi/api"
)

type Database interface {
	CreatePusher(ctx context.Context, sessionId int64, pushkey, kind, appid, appdisplayname, devicedisplayname, profiletag, lang, data, localpart string) error
	GetPushersByLocalpart(ctx context.Context, localpart string) ([]api.Pusher, error)
	GetPusherByPushkey(ctx context.Context, pushkey, localpart string) (*api.Pusher, error)
	UpdatePusher(ctx context.Context, pushkey, kind, appid, appdisplayname, devicedisplayname, profiletag, lang, data, localpart string) error
	RemovePusher(ctx context.Context, appId, pushkey, localpart string) error
}
