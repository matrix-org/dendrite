// Copyright 2017 Thibaut CHARLES
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

package postgres

import (
	"strings"

	"github.com/matrix-org/gomatrixserverlib"
)

// filterConvertWildcardToSQL converts wildcards as defined in
// https://matrix.org/docs/spec/client_server/r0.3.0.html#post-matrix-client-r0-user-userid-filter
// to SQL wildcards that can be used with LIKE()
func filterConvertTypeWildcardToSQL(values *[]string) []string {
	if values == nil {
		// Return nil instead of []string{} so IS NULL can work correctly when
		// the return value is passed into SQL queries
		return nil
	}

	v := *values
	ret := make([]string, len(v))
	for i := range v {
		ret[i] = strings.Replace(v[i], "*", "%", -1)
	}
	return ret
}

// TODO: Replace when Dendrite uses Go 1.18
func getSendersRoomEventFilter(filter *gomatrixserverlib.RoomEventFilter) (senders []string, notSenders []string) {
	if filter.Senders != nil {
		senders = *filter.Senders
	}
	if filter.NotSenders != nil {
		notSenders = *filter.NotSenders
	}
	return senders, notSenders
}

func getSendersStateFilterFilter(filter *gomatrixserverlib.StateFilter) (senders []string, notSenders []string) {
	if filter.Senders != nil {
		senders = *filter.Senders
	}
	if filter.NotSenders != nil {
		notSenders = *filter.NotSenders
	}
	return senders, notSenders
}
