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
	"io"
	"io/ioutil"
	"os"
	"strings"

	"github.com/sirupsen/logrus"
	"golang.org/x/crypto/bcrypt"
	"golang.org/x/term"

	"github.com/matrix-org/dendrite/setup"
	"github.com/matrix-org/dendrite/setup/config"
	"github.com/matrix-org/dendrite/userapi/api"
	userdb "github.com/matrix-org/dendrite/userapi/storage"
)

const usage = `Usage: %s

Creates a new user account on the homeserver.

Example:

	# provide password by parameter
  	%s --config dendrite.yaml -username alice -password foobarbaz
	# use password from file
  	%s --config dendrite.yaml -username alice -passwordfile my.pass
	# ask user to provide password
	%s --config dendrite.yaml -username alice -ask-pass
	# read password from stdin
	%s --config dendrite.yaml -username alice -passwordstdin < my.pass
	cat my.pass | %s --config dendrite.yaml -username alice -passwordstdin
	# reset password for a user, can be used with a combination above to read the password
	%s --config dendrite.yaml -reset-password -username alice -password foobarbaz

Arguments:

`

var (
	username      = flag.String("username", "", "The username of the account to register (specify the localpart only, e.g. 'alice' for '@alice:domain.com')")
	password      = flag.String("password", "", "The password to associate with the account (optional, account will be password-less if not specified)")
	pwdFile       = flag.String("passwordfile", "", "The file to use for the password (e.g. for automated account creation)")
	pwdStdin      = flag.Bool("passwordstdin", false, "Reads the password from stdin")
	askPass       = flag.Bool("ask-pass", false, "Ask for the password to use")
	isAdmin       = flag.Bool("admin", false, "Create an admin account")
	resetPassword = flag.Bool("reset-password", false, "Resets the password for the given username")
)

func main() {
	name := os.Args[0]
	flag.Usage = func() {
		_, _ = fmt.Fprintf(os.Stderr, usage, name, name, name, name, name, name, name)
		flag.PrintDefaults()
	}
	cfg := setup.ParseFlags(true)

	if *username == "" {
		flag.Usage()
		os.Exit(1)
	}

	pass := getPassword(password, pwdFile, pwdStdin, askPass, os.Stdin)

	accountDB, err := userdb.NewDatabase(
		&config.DatabaseOptions{
			ConnectionString: cfg.UserAPI.AccountDatabase.ConnectionString,
		},
		cfg.Global.ServerName, bcrypt.DefaultCost,
		cfg.UserAPI.OpenIDTokenLifetimeMS,
		api.DefaultLoginTokenLifetime,
	)
	if err != nil {
		logrus.Fatalln("Failed to connect to the database:", err.Error())
	}

	accType := api.AccountTypeUser
	if *isAdmin {
		accType = api.AccountTypeAdmin
	}

	if *resetPassword {
		err = accountDB.SetPassword(context.Background(), *username, pass)
		if err != nil {
			logrus.Fatalf("Failed to update password for user %s: %s", *username, err.Error())
		}
		if _, err = accountDB.RemoveAllDevices(context.Background(), *username, ""); err != nil {
			logrus.Fatalf("Failed to remove all devices: %s", err.Error())
		}
		logrus.Infof("Updated password for user %s and invalidated all logins\n", *username)
		return
	}

	_, err = accountDB.CreateAccount(context.Background(), *username, pass, "", accType)
	if err != nil {
		logrus.Fatalln("Failed to create the account:", err.Error())
	}

	logrus.Infoln("Created account", *username)
}

func getPassword(password, pwdFile *string, pwdStdin, askPass *bool, r io.Reader) string {
	// no password option set, use empty password
	if password == nil && pwdFile == nil && pwdStdin == nil && askPass == nil {
		return ""
	}
	// password defined as parameter
	if password != nil && *password != "" {
		return *password
	}

	// read password from file
	if pwdFile != nil && *pwdFile != "" {
		pw, err := ioutil.ReadFile(*pwdFile)
		if err != nil {
			logrus.Fatalln("Unable to read password from file:", err)
		}
		return strings.TrimSpace(string(pw))
	}

	// read password from stdin
	if pwdStdin != nil && *pwdStdin {
		data, err := ioutil.ReadAll(r)
		if err != nil {
			logrus.Fatalln("Unable to read password from stdin:", err)
		}
		return strings.TrimSpace(string(data))
	}

	// ask the user to provide the password
	if *askPass {
		fmt.Print("Enter Password: ")
		bytePassword, err := term.ReadPassword(int(os.Stdin.Fd()))
		if err != nil {
			logrus.Fatalln("Unable to read password:", err)
		}
		fmt.Println()
		fmt.Print("Confirm Password: ")
		bytePassword2, err := term.ReadPassword(int(os.Stdin.Fd()))
		if err != nil {
			logrus.Fatalln("Unable to read password:", err)
		}
		fmt.Println()
		if strings.TrimSpace(string(bytePassword)) != strings.TrimSpace(string(bytePassword2)) {
			logrus.Fatalln("Entered passwords don't match")
		}
		return strings.TrimSpace(string(bytePassword))
	}

	return ""
}
