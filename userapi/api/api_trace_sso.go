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

package api

import (
	"context"

	"github.com/matrix-org/util"
)

func (t *UserInternalAPITrace) QueryLocalpartForSSO(ctx context.Context, req *QueryLocalpartForSSORequest, res *QueryLocalpartForSSOResponse) error {
	err := t.Impl.QueryLocalpartForSSO(ctx, req, res)
	util.GetLogger(ctx).Infof("QueryLocalpartForSSO req=%+v res=%+v", js(req), js(res))
	return err
}

func (t *UserInternalAPITrace) PerformForgetSSO(ctx context.Context, req *PerformForgetSSORequest, res *struct{}) error {
	err := t.Impl.PerformForgetSSO(ctx, req, res)
	util.GetLogger(ctx).Infof("PerformForgetSSO req=%+v res=%+v", js(req), js(res))
	return err
}

func (t *UserInternalAPITrace) PerformSaveSSOAssociation(ctx context.Context, req *PerformSaveSSOAssociationRequest, res *struct{}) error {
	err := t.Impl.PerformSaveSSOAssociation(ctx, req, res)
	util.GetLogger(ctx).Infof("PerformSaveSSOAssociation req=%+v res=%+v", js(req), js(res))
	return err
}
