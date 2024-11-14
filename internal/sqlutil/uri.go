// Copyright 2024 New Vector Ltd.
// Copyright 2020 The Matrix.org Foundation C.I.C.
//
// SPDX-License-Identifier: AGPL-3.0-only OR LicenseRef-Element-Commercial
// Please see LICENSE files in the repository root for full details.

package sqlutil

import (
	"errors"
	"fmt"
	"net/url"

	"github.com/element-hq/dendrite/setup/config"
)

// ParseFileURI returns the filepath in the given file: URI. Specifically, this will handle
// both relative (file:foo.db) and absolute (file:///path/to/foo) paths.
func ParseFileURI(dataSourceName config.DataSource) (string, error) {
	if !dataSourceName.IsSQLite() {
		return "", errors.New("ParseFileURI expects SQLite connection string")
	}
	uri, err := url.Parse(string(dataSourceName))
	if err != nil {
		return "", err
	}
	var cs string
	if uri.Opaque != "" { // file:filename.db
		cs = uri.Opaque
	} else if uri.Path != "" { // file:///path/to/filename.db
		cs = uri.Path
	} else {
		return "", fmt.Errorf("invalid file uri: %s", dataSourceName)
	}
	return cs, nil
}
