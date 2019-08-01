// Copyright 2019 Alex Chen
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

package common

import "github.com/matrix-org/gomatrixserverlib"

func ValidateRedaction(
	redacted, redaction *gomatrixserverlib.Event,
) (badEvents, needPowerLevelCheck bool, err error) {
	// Don't allow redaction of events in different rooms
	if redaction.RoomID() != redacted.RoomID() {
		return true, false, nil
	}

	var expectedDomain, redactorDomain gomatrixserverlib.ServerName
	if _, expectedDomain, err = gomatrixserverlib.SplitID(
		'@', redacted.Sender(),
	); err != nil {
		return false, false, err
	}
	if _, redactorDomain, err = gomatrixserverlib.SplitID(
		'@', redaction.Sender(),
	); err != nil {
		return false, false, err
	}

	if expectedDomain != redactorDomain {
		return false, true, err
	}

	return false, false, nil
}
