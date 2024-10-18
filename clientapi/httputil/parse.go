// Copyright 2024 New Vector Ltd.
//
// SPDX-License-Identifier: AGPL-3.0-only OR LicenseRef-Element-Commercial
// Please see LICENSE files in the repository root for full details.

package httputil

import (
	"fmt"
	"net/http"
	"strconv"
	"time"
)

// ParseTSParam takes a req (typically from an application service) and parses a Time object
// from the req if it exists in the query parameters. If it doesn't exist, the
// current time is returned.
func ParseTSParam(req *http.Request) (time.Time, error) {
	// Use the ts parameter's value for event time if present
	tsStr := req.URL.Query().Get("ts")
	if tsStr == "" {
		return time.Now(), nil
	}

	// The parameter exists, parse into a Time object
	ts, err := strconv.ParseInt(tsStr, 10, 64)
	if err != nil {
		return time.Time{}, fmt.Errorf("param 'ts' is no valid int (%s)", err.Error())
	}

	return time.UnixMilli(ts), nil
}
