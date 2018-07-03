// Copyright 2018 Vector Creations Ltd
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

package encryptoapi

import (
	"github.com/matrix-org/dendrite/common/basecomponent"
	"github.com/matrix-org/dendrite/clientapi/auth/storage/accounts"
	"github.com/matrix-org/dendrite/encryptoapi/routing"
	"github.com/matrix-org/dendrite/clientapi/auth/storage/devices"
	"github.com/matrix-org/dendrite/encryptoapi/storage"
	"fmt"
)

// in order to gain key management capability
// , CMD should involve this invoke into main function
// , a setup need an assemble of i.e configs as base and
// accountDB and deviceDB
func SetupEcryptoapi(
	base *basecomponent.BaseDendrite,
	accountsDB *accounts.Database,
	deviceDB *devices.Database,
) {

	encryptionDB, err := storage.NewDatabase(string(base.Cfg.Database.EncryptAPI))
	fmt.Print(err)
	routing.Setup(
		base.APIMux,
		*base.Cfg,
		encryptionDB,
		accountsDB,
		deviceDB,
	)
	routing.InitNotifier(base)

}
