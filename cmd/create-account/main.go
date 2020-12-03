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

package main

import (
	"context"
	"flag"
	"fmt"
	"os"

	"github.com/matrix-org/dendrite/setup"
	"github.com/matrix-org/dendrite/setup/config"
	"github.com/matrix-org/dendrite/userapi/storage/accounts"
	"github.com/sirupsen/logrus"
)

const usage = `Usage: %s

Creates a new user account on the homeserver.

Example:

  ./create-account --config dendrite.yaml --username alice --password foobarbaz

Arguments:

`

var (
	username = flag.String("username", "", "The username of the account to register (specify the localpart only, e.g. 'alice' for '@alice:domain.com')")
	password = flag.String("password", "", "The password to associate with the account (optional, account will be password-less if not specified)")
)

func main() {
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, usage, os.Args[0])
		flag.PrintDefaults()
	}
	cfg := setup.ParseFlags(true)

	if *username == "" {
		flag.Usage()
		os.Exit(1)
	}

	accountDB, err := accounts.NewDatabase(&config.DatabaseOptions{
		ConnectionString: cfg.UserAPI.AccountDatabase.ConnectionString,
	}, cfg.Global.ServerName)
	if err != nil {
		logrus.Fatalln("Failed to connect to the database:", err.Error())
	}

	_, err = accountDB.CreateAccount(context.Background(), *username, *password, "")
	if err != nil {
		logrus.Fatalln("Failed to create the account:", err.Error())
	}

	logrus.Infoln("Created account", *username)
}
