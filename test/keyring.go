// Copyright 2024 New Vector Ltd.
// Copyright 2020 The Matrix.org Foundation C.I.C.
//
// SPDX-License-Identifier: AGPL-3.0-only OR LicenseRef-Element-Commercial
// Please see LICENSE files in the repository root for full details.

package test

import (
	"context"

	"github.com/matrix-org/gomatrixserverlib"
)

// NopJSONVerifier is a JSONVerifier that verifies nothing and returns no errors.
type NopJSONVerifier struct {
	// this verifier verifies nothing
}

func (t *NopJSONVerifier) VerifyJSONs(ctx context.Context, requests []gomatrixserverlib.VerifyJSONRequest) ([]gomatrixserverlib.VerifyJSONResult, error) {
	result := make([]gomatrixserverlib.VerifyJSONResult, len(requests))
	return result, nil
}
