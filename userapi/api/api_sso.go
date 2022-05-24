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
)

type SSOAPI interface {
	QueryLocalpartForSSO(ctx context.Context, req *QueryLocalpartForSSORequest, res *QueryLocalpartForSSOResponse) error
	PerformForgetSSO(ctx context.Context, req *PerformForgetSSORequest, res *struct{}) error
	PerformSaveSSOAssociation(ctx context.Context, req *PerformSaveSSOAssociationRequest, res *struct{}) error
}

type QueryLocalpartForSSORequest struct {
	Namespace       SSOIssuerNamespace
	Issuer, Subject string
}

type QueryLocalpartForSSOResponse struct {
	Localpart string
}

type PerformForgetSSORequest QueryLocalpartForSSORequest

type PerformSaveSSOAssociationRequest struct {
	Namespace       SSOIssuerNamespace
	Issuer, Subject string
	Localpart       string
}

// An SSOIssuerNamespace defines the interpretation of an issuer.
type SSOIssuerNamespace string

const (
	UnknownNamespace SSOIssuerNamespace = ""

	// SSOIDNamespace indicates the issuer is an ID key matching a
	// Dendrite SSO provider configuration.
	SSOIDNamespace SSOIssuerNamespace = "sso"

	// OIDCNamespace indicates the issuer is a full URL, as defined in
	// https://openid.net/specs/openid-connect-core-1_0.html#Terminology.
	OIDCNamespace SSOIssuerNamespace = "oidc"
)
