/* Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

package gomatrixserverlib

// EDU represents a EDU received via federation
// https://matrix.org/docs/spec/server_server/unstable.html#edus
type EDU struct {
	Type        string  `json:"edu_type"`
	Origin      string  `json:"origin"`
	Destination string  `json:"destination"`
	Content     RawJSON `json:"content"`
}
