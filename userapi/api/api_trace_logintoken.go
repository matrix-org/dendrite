// Copyright 2021 The Matrix.org Foundation C.I.C.
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

package api

import (
	"context"

	"github.com/matrix-org/util"
)

func (t *UserInternalAPITrace) PerformLoginTokenCreation(ctx context.Context, req *PerformLoginTokenCreationRequest, res *PerformLoginTokenCreationResponse) error {
	err := t.Impl.PerformLoginTokenCreation(ctx, req, res)
	util.GetLogger(ctx).Infof("PerformLoginTokenCreation req=%+v res=%+v", js(req), js(res))
	return err
}

func (t *UserInternalAPITrace) PerformLoginTokenDeletion(ctx context.Context, req *PerformLoginTokenDeletionRequest, res *PerformLoginTokenDeletionResponse) error {
	err := t.Impl.PerformLoginTokenDeletion(ctx, req, res)
	util.GetLogger(ctx).Infof("PerformLoginTokenDeletion req=%+v res=%+v", js(req), js(res))
	return err
}

func (t *UserInternalAPITrace) QueryLoginToken(ctx context.Context, req *QueryLoginTokenRequest, res *QueryLoginTokenResponse) error {
	err := t.Impl.QueryLoginToken(ctx, req, res)
	util.GetLogger(ctx).Infof("QueryLoginToken req=%+v res=%+v", js(req), js(res))
	return err
}
