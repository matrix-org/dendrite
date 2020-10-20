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

	"github.com/matrix-org/dendrite/internal/config"
	"github.com/matrix-org/dendrite/userapi/storage/accounts"
	"github.com/matrix-org/gomatrixserverlib"
)

const usage = `Usage: %s

Generate a new Matrix account for testing purposes.

Arguments:

`

var (
	database      = flag.String("database", "", "The location of the account database.")
	username      = flag.String("username", "", "The user ID localpart to register e.g 'alice' in '@alice:localhost'.")
	password      = flag.String("password", "", "Optional. The password to register with. If not specified, this account will be password-less.")
	serverNameStr = flag.String("servername", "localhost", "The Matrix server domain which will form the domain part of the user ID.")
)

func main() {
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, usage, os.Args[0])
		flag.PrintDefaults()
	}

	flag.Parse()

	if *username == "" {
		flag.Usage()
		fmt.Println("Missing --username")
		os.Exit(1)
	}

	if *database == "" {
		flag.Usage()
		fmt.Println("Missing --database")
		os.Exit(1)
	}

	serverName := gomatrixserverlib.ServerName(*serverNameStr)

	accountDB, err := accounts.NewDatabase(&config.DatabaseOptions{
		ConnectionString: config.DataSource(*database),
	}, serverName)
	if err != nil {
		fmt.Println(err.Error())
		os.Exit(1)
	}

	_, err = accountDB.CreateAccount(context.Background(), *username, *password, "")
	if err != nil {
		fmt.Println(err.Error())
		os.Exit(1)
	}

	fmt.Println("Created account")
}
