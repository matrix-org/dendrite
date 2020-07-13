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

package api

import "context"

type KeyInternalAPI interface {
	PerformUploadKeys(ctx context.Context, req *PerformUploadKeysRequest, res *PerformUploadKeysResponse)
	PerformClaimKeys(ctx context.Context, req *PerformClaimKeysRequest, res *PerformClaimKeysResponse)
	QueryKeys(ctx context.Context, req *QueryKeysRequest, res *QueryKeysResponse)
}

// KeyError is returned if there was a problem performing/querying the server
type KeyError struct {
	Error string
}

type PerformUploadKeysRequest struct {
}

type PerformUploadKeysResponse struct {
	Error *KeyError
}

type PerformClaimKeysRequest struct {
}

type PerformClaimKeysResponse struct {
	Error *KeyError
}

type QueryKeysRequest struct {
}

type QueryKeysResponse struct {
	Error *KeyError
}
