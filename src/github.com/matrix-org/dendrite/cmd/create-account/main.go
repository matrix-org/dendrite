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
	"flag"
	"fmt"
	"os"

	"github.com/matrix-org/dendrite/clientapi/auth/storage/accounts"
	"github.com/matrix-org/dendrite/clientapi/auth/storage/devices"
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
	accessToken   = flag.String("token", "", "Optional. The desired access_token to have. If not specified, a random access_token will be made.")
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

	accountDB, err := accounts.NewDatabase(*database, serverName)
	if err != nil {
		fmt.Println(err.Error())
		os.Exit(1)
	}

	_, err = accountDB.CreateAccount(*username, *password)
	if err != nil {
		fmt.Println(err.Error())
		os.Exit(1)
	}

	deviceDB, err := devices.NewDatabase(*database, serverName)
	if err != nil {
		fmt.Println(err.Error())
		os.Exit(1)
	}

	if *accessToken == "" {
		t := "token_" + *username
		accessToken = &t
	}

	device, err := deviceDB.CreateDevice(*username, "create-account-script", *accessToken)
	if err != nil {
		fmt.Println(err.Error())
		os.Exit(1)
	}

	fmt.Println("Created account:")
	fmt.Printf("user_id      = %s\n", device.UserID)
	fmt.Printf("device_id    = %s\n", device.ID)
	fmt.Printf("access_token = %s\n", device.AccessToken)
}
