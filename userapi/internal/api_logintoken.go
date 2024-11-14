// Copyright 2024 New Vector Ltd.
// Copyright 2021 The Matrix.org Foundation C.I.C.
//
// SPDX-License-Identifier: AGPL-3.0-only OR LicenseRef-Element-Commercial
// Please see LICENSE files in the repository root for full details.

package internal

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/element-hq/dendrite/userapi/api"
	"github.com/matrix-org/gomatrixserverlib"
	"github.com/matrix-org/util"
)

// PerformLoginTokenCreation creates a new login token and associates it with the provided data.
func (a *UserInternalAPI) PerformLoginTokenCreation(ctx context.Context, req *api.PerformLoginTokenCreationRequest, res *api.PerformLoginTokenCreationResponse) error {
	util.GetLogger(ctx).WithField("user_id", req.Data.UserID).Info("PerformLoginTokenCreation")
	_, domain, err := gomatrixserverlib.SplitID('@', req.Data.UserID)
	if err != nil {
		return err
	}
	if !a.Config.Matrix.IsLocalServerName(domain) {
		return fmt.Errorf("cannot create a login token for a remote user (server name %s)", domain)
	}
	tokenMeta, err := a.DB.CreateLoginToken(ctx, &req.Data)
	if err != nil {
		return err
	}
	res.Metadata = *tokenMeta
	return nil
}

// PerformLoginTokenDeletion ensures the token doesn't exist.
func (a *UserInternalAPI) PerformLoginTokenDeletion(ctx context.Context, req *api.PerformLoginTokenDeletionRequest, res *api.PerformLoginTokenDeletionResponse) error {
	util.GetLogger(ctx).Info("PerformLoginTokenDeletion")
	return a.DB.RemoveLoginToken(ctx, req.Token)
}

// QueryLoginToken returns the data associated with a login token. If
// the token is not valid, success is returned, but res.Data == nil.
func (a *UserInternalAPI) QueryLoginToken(ctx context.Context, req *api.QueryLoginTokenRequest, res *api.QueryLoginTokenResponse) error {
	tokenData, err := a.DB.GetLoginTokenDataByToken(ctx, req.Token)
	if err != nil {
		res.Data = nil
		if err == sql.ErrNoRows {
			return nil
		}
		return err
	}
	localpart, domain, err := gomatrixserverlib.SplitID('@', tokenData.UserID)
	if err != nil {
		return err
	}
	if !a.Config.Matrix.IsLocalServerName(domain) {
		return fmt.Errorf("cannot return a login token for a remote user (server name %s)", domain)
	}
	if _, err := a.DB.GetAccountByLocalpart(ctx, localpart, domain); err != nil {
		res.Data = nil
		if err == sql.ErrNoRows {
			return nil
		}
		return err
	}
	res.Data = tokenData
	return nil
}
