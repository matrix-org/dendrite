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

package internal

import (
	"context"
	"fmt"

	"github.com/matrix-org/dendrite/userapi/api"
)

func (a *UserInternalAPI) QueryLocalpartForSSO(ctx context.Context, req *api.QueryLocalpartForSSORequest, res *api.QueryLocalpartForSSOResponse) error {
	var err error
	res.Localpart, err = a.DB.GetLocalpartForSSO(ctx, string(req.Namespace), req.Issuer, req.Subject)
	return err
}

func (a *UserInternalAPI) PerformForgetSSO(ctx context.Context, req *api.PerformForgetSSORequest, res *struct{}) error {
	return a.DB.RemoveSSOAssociation(ctx, string(req.Namespace), req.Issuer, req.Subject)
}

func (a *UserInternalAPI) PerformSaveSSOAssociation(ctx context.Context, req *api.PerformSaveSSOAssociationRequest, res *struct{}) error {
	ns, err := validateSSOIssuerNamespace(req.Namespace)
	if err != nil {
		return err
	}
	return a.DB.SaveSSOAssociation(ctx, ns, req.Issuer, req.Subject, req.Localpart)
}

func validateSSOIssuerNamespace(ns api.SSOIssuerNamespace) (string, error) {
	switch ns {
	case api.OIDCNamespace:
		return string(ns), nil

	default:
		return "", fmt.Errorf("invalid SSO issuer namespace: %s", ns)
	}
}
