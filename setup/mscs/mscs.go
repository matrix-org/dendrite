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

// Package mscs implements Matrix Spec Changes from https://github.com/matrix-org/matrix-doc
package mscs

import (
	"context"
	"fmt"

	"github.com/matrix-org/dendrite/setup"
	"github.com/matrix-org/dendrite/setup/base"
	"github.com/matrix-org/dendrite/setup/mscs/msc2836"
	"github.com/matrix-org/dendrite/setup/mscs/msc2946"
	"github.com/matrix-org/util"
)

// Enable MSCs - returns an error on unknown MSCs
func Enable(base *base.BaseDendrite, monolith *setup.Monolith) error {
	for _, msc := range base.Cfg.MSCs.MSCs {
		util.GetLogger(context.Background()).WithField("msc", msc).Info("Enabling MSC")
		if err := EnableMSC(base, monolith, msc); err != nil {
			return err
		}
	}
	return nil
}

func EnableMSC(base *base.BaseDendrite, monolith *setup.Monolith, msc string) error {
	switch msc {
	case "msc2836":
		return msc2836.Enable(base, monolith.RoomserverAPI, monolith.FederationAPI, monolith.UserAPI, monolith.KeyRing)
	case "msc2946":
		return msc2946.Enable(base, monolith.RoomserverAPI, monolith.UserAPI, monolith.FederationAPI, monolith.KeyRing, base.Caches)
	case "msc2444": // enabled inside federationapi
	case "msc2753": // enabled inside clientapi
	default:
		return fmt.Errorf("EnableMSC: unknown msc '%s'", msc)
	}
	return nil
}
