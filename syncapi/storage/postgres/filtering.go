// Copyright 2024 New Vector Ltd.
// Copyright 2017 Thibaut CHARLES
//
// SPDX-License-Identifier: AGPL-3.0-only OR LicenseRef-Element-Commercial
// Please see LICENSE files in the repository root for full details.

package postgres

import (
	"strings"

	"github.com/element-hq/dendrite/syncapi/synctypes"
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
func getSendersRoomEventFilter(filter *synctypes.RoomEventFilter) (senders []string, notSenders []string) {
	if filter.Senders != nil {
		senders = *filter.Senders
	}
	if filter.NotSenders != nil {
		notSenders = *filter.NotSenders
	}
	return senders, notSenders
}

func getSendersStateFilterFilter(filter *synctypes.StateFilter) (senders []string, notSenders []string) {
	if filter.Senders != nil {
		senders = *filter.Senders
	}
	if filter.NotSenders != nil {
		notSenders = *filter.NotSenders
	}
	return senders, notSenders
}
