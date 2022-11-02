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

package types

type UserStatistics struct {
	RegisteredUsersByType map[string]int64
	R30Users              map[string]int64
	R30UsersV2            map[string]int64
	AllUsers              int64
	NonBridgedUsers       int64
	DailyUsers            int64
	MonthlyUsers          int64
}

type DatabaseEngine struct {
	Engine  string
	Version string
}

type MessageStats struct {
	Messages         int64
	SentMessages     int64
	MessagesE2EE     int64
	SentMessagesE2EE int64
}
