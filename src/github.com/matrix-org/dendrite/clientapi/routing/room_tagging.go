// Copyright 2017 Vector Creations Ltd
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

package routing

import "net/http"

// GetTag implements GET /_matrix/client/r0/user/{userId}/rooms/{roomId}/tags
func GetTag(req *http.Request, userId string, roomId string) {

}

// PutTag implements GET /_matrix/client/r0/user/{userId}/rooms/{roomId}/tags/{tag}
func PutTag(req *http.Request, userId string, roomId string, tag string) {

}

// DeleteTag implements DELETE /_matrix/client/r0/user/{userId}/rooms/{roomId}/tags/{tag}
func DeleteTag(req *http.Request, userId string, roomId string, tag string) {

}
