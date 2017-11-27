// Copyright 2017 Vector Creations Ltd
// Copyright 2017 New Vector Ltd
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

import (
	"fmt"
	"testing"

	"github.com/matrix-org/dendrite/clientapi/auth/authtypes"
)

func TestFlowChecking(t *testing.T) {
	derivedFlows := []authtypes.Flow{
		{
			[]authtypes.LoginType{
				authtypes.LoginType("type1"),
				authtypes.LoginType("type2"),
			},
		},
		{
			[]authtypes.LoginType{
				authtypes.LoginType("type1"),
				authtypes.LoginType("type3"),
			},
		},
	}

	testFlow1 := authtypes.Flow{
		[]authtypes.LoginType{
			authtypes.LoginType("type1"),
			authtypes.LoginType("type3"),
		},
	}
	testFlow2 := authtypes.Flow{
		[]authtypes.LoginType{
			authtypes.LoginType("type2"),
			authtypes.LoginType("type3"),
		},
	}
	testFlow3 := authtypes.Flow{
		[]authtypes.LoginType{
			authtypes.LoginType("type1"),
			authtypes.LoginType("type3"),
			authtypes.LoginType("type4"),
		},
	}
	testFlow4 := authtypes.Flow{
		[]authtypes.LoginType{},
	}
	testFlow5 := authtypes.Flow{
		[]authtypes.LoginType{
			authtypes.LoginType("type3"),
			authtypes.LoginType("type2"),
			authtypes.LoginType("type1"),
		},
	}

	if !checkFlowCompleted(testFlow1, derivedFlows) {
		t.Error(fmt.Sprint("Failed to verify registration flow: ", testFlow1, ", from derived flows: ", derivedFlows, ". Should be true."))
	}
	if checkFlowCompleted(testFlow2, derivedFlows) {
		t.Error(fmt.Sprint("Failed to verify registration flow: ", testFlow2, ", from derived flows: ", derivedFlows, ". Should be false."))
	}
	if !checkFlowCompleted(testFlow3, derivedFlows) {
		t.Error(fmt.Sprint("Failed to verify registration flow: ", testFlow3, ", from derived flows: ", derivedFlows, ". Should be true."))
	}
	if checkFlowCompleted(testFlow4, derivedFlows) {
		t.Error(fmt.Sprint("Failed to verify registration flow: ", testFlow4, ", from derived flows: ", derivedFlows, ". Should be false."))
	}
	if !checkFlowCompleted(testFlow5, derivedFlows) {
		t.Error(fmt.Sprint("Failed to verify registration flow: ", testFlow5, ", from derived flows: ", derivedFlows, ". Should be true."))
	}
}
