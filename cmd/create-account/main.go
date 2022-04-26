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
	"regexp"
	"strings"

	"github.com/matrix-org/dendrite/setup"
	"github.com/matrix-org/dendrite/setup/base"
	"github.com/matrix-org/dendrite/userapi/api"
	"github.com/sirupsen/logrus"
	"golang.org/x/term"
)

const usage = `Usage: %s

Creates a new user account on the homeserver.

Example:

	# provide password by parameter
  	%s --config dendrite.yaml -username alice -password foobarbaz
	# use password from file
  	%s --config dendrite.yaml -username alice -passwordfile my.pass
	# ask user to provide password
	%s --config dendrite.yaml -username alice
	# read password from stdin
	%s --config dendrite.yaml -username alice -passwordstdin < my.pass
	cat my.pass | %s --config dendrite.yaml -username alice -passwordstdin
	# reset password for a user, can be used with a combination above to read the password
	%s --config dendrite.yaml -reset-password -username alice -password foobarbaz

Arguments:

`

var (
	username           = flag.String("username", "", "The username of the account to register (specify the localpart only, e.g. 'alice' for '@alice:domain.com')")
	password           = flag.String("password", "", "The password to associate with the account")
	pwdFile            = flag.String("passwordfile", "", "The file to use for the password (e.g. for automated account creation)")
	pwdStdin           = flag.Bool("passwordstdin", false, "Reads the password from stdin")
	pwdLess            = flag.Bool("passwordless", false, "Create a passwordless account, e.g. if only an accesstoken is required")
	isAdmin            = flag.Bool("admin", false, "Create an admin account")
	resetPassword      = flag.Bool("reset-password", false, "Resets the password for the given username")
	validUsernameRegex = regexp.MustCompile(`^[0-9a-z_\-=./]+$`)
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

	if *pwdLess && *resetPassword {
		logrus.Fatalf("Can not reset to an empty password, unable to login afterwards.")
	}

	if !validUsernameRegex.MatchString(*username) {
		logrus.Warn("Username can only contain characters a-z, 0-9, or '_-./='")
		os.Exit(1)
	}

	if len(fmt.Sprintf("@%s:%s", *username, cfg.Global.ServerName)) > 255 {
		logrus.Fatalf("Username can not be longer than 255 characters: %s", fmt.Sprintf("@%s:%s", *username, cfg.Global.ServerName))
	}

	var pass string
	var err error
	if !*pwdLess {
		pass, err = getPassword(*password, *pwdFile, *pwdStdin, os.Stdin)
		if err != nil {
			logrus.Fatalln(err)
		}
	}

	b := base.NewBaseDendrite(cfg, "Monolith")
	accountDB := b.CreateAccountsDB()

	accType := api.AccountTypeUser
	if *isAdmin {
		accType = api.AccountTypeAdmin
	}

	available, err := accountDB.CheckAccountAvailability(context.Background(), *username)
	if err != nil {
		logrus.Fatalln("Unable check username existence.")
	}
	if *resetPassword {
		if available {
			logrus.Fatalln("Username could not be found.")
		}
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
	if !available {
		logrus.Fatalln("Username is already in use.")
	}

	_, err = accountDB.CreateAccount(context.Background(), *username, pass, "", accType)
	if err != nil {
		logrus.Fatalln("Failed to create the account:", err.Error())
	}

	logrus.Infoln("Created account", *username)
}

func getPassword(password, pwdFile string, pwdStdin bool, r io.Reader) (string, error) {
	// read password from file
	if pwdFile != "" {
		pw, err := ioutil.ReadFile(pwdFile)
		if err != nil {
			return "", fmt.Errorf("Unable to read password from file: %v", err)
		}
		return strings.TrimSpace(string(pw)), nil
	}

	// read password from stdin
	if pwdStdin {
		data, err := ioutil.ReadAll(r)
		if err != nil {
			return "", fmt.Errorf("Unable to read password from stdin: %v", err)
		}
		return strings.TrimSpace(string(data)), nil
	}

	// If no parameter was set, ask the user to provide the password
	if password == "" {
		fmt.Print("Enter Password: ")
		bytePassword, err := term.ReadPassword(int(os.Stdin.Fd()))
		if err != nil {
			return "", fmt.Errorf("Unable to read password: %v", err)
		}
		fmt.Println()
		fmt.Print("Confirm Password: ")
		bytePassword2, err := term.ReadPassword(int(os.Stdin.Fd()))
		if err != nil {
			return "", fmt.Errorf("Unable to read password: %v", err)
		}
		fmt.Println()
		if strings.TrimSpace(string(bytePassword)) != strings.TrimSpace(string(bytePassword2)) {
			return "", fmt.Errorf("Entered passwords don't match")
		}
		return strings.TrimSpace(string(bytePassword)), nil
	}

	return password, nil
}
