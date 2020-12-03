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

package sqlutil

import (
	"errors"
	"fmt"
	"net/url"

	"github.com/matrix-org/dendrite/setup/config"
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
