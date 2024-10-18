// Copyright 2017-2024 New Vector Ltd.
//
// SPDX-License-Identifier: AGPL-3.0-only OR LicenseRef-Element-Commercial
// Please see LICENSE files in the repository root for full details.

package routing

import (
	"net/http"

	"github.com/element-hq/dendrite/internal"
	"github.com/matrix-org/util"
)

type version struct {
	Server server `json:"server"`
}

type server struct {
	Version string `json:"version"`
	Name    string `json:"name"`
}

// Version returns the server version
func Version() util.JSONResponse {
	return util.JSONResponse{
		Code: http.StatusOK,
		JSON: &version{
			server{
				Name:    "Dendrite",
				Version: internal.VersionString(),
			},
		},
	}
}
