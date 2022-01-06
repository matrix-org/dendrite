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
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"flag"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"strings"

	"github.com/matrix-org/dendrite/setup"
	"github.com/matrix-org/dendrite/setup/config"
	"github.com/matrix-org/dendrite/userapi/storage/accounts"
	"github.com/sirupsen/logrus"
	"golang.org/x/crypto/bcrypt"
	"golang.org/x/term"
)

const usage = `Usage: %s

Creates a new user account on the homeserver.

Example:

	# provide password by parameter
  	%s --config dendrite.yaml -username alice -password foobarbaz
	# auto generate keypair
  	%s --config dendrite.yaml -username alice -password foobarbaz -udk NuJ7J4BsaE8QZT1ULNTc3s8ZjLFmDPh91l1i0Urf/ls=
	# use password from file
  	%s --config dendrite.yaml -username alice -passwordfile my.pass
	# ask user to provide password
	%s --config dendrite.yaml -username alice -ask-pass
	# read password from stdin
	%s --config dendrite.yaml -username alice -passwordstdin < my.pass
	cat my.pass | %s --config dendrite.yaml -username alice -passwordstdin

Arguments:

`

var (
	username      = flag.String("username", "", "The username of the account to register (specify the localpart only, e.g. 'alice' for '@alice:domain.com')")
	password      = flag.String("password", "", "The password to associate with the account (optional, account will be password-less if not specified)")
	createKeypair = flag.Bool("create-keypair", false, "Whether to create an Ed25519 keypair for the account to create (optional)")
	pwdFile       = flag.String("passwordfile", "", "The file to use for the password (e.g. for automated account creation)")
	pwdStdin      = flag.Bool("passwordstdin", false, "Reads the password from stdin")
	askPass       = flag.Bool("ask-pass", false, "Ask for the password to use")
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

	accountDB, err := accounts.NewDatabase(&config.DatabaseOptions{
		ConnectionString: cfg.UserAPI.AccountDatabase.ConnectionString,
	}, cfg.Global.ServerName, bcrypt.DefaultCost, cfg.UserAPI.OpenIDTokenLifetimeMS)
	if err != nil {
		logrus.Fatalln("Failed to connect to the database:", err.Error())
	}

	var pub64 string
	if *createKeypair {
		pub, priv, err2 := ed25519.GenerateKey(rand.Reader)
		pub64 = base64.StdEncoding.EncodeToString(priv.Public().(ed25519.PublicKey))
		if err2 != nil {
			logrus.Fatalln(err2)
		}
		err2 = ioutil.WriteFile("private.key", priv, 0644)
		if err2 != nil {
			logrus.Fatalln(err2)
		}
		err2 = ioutil.WriteFile("private.key.seed", priv.Seed(), 0644)
		if err2 != nil {
			logrus.Fatalln(err2)
		}
		err2 = ioutil.WriteFile("./public.key", pub, 0644)
		if err2 != nil {
			logrus.Fatalln(err2)
		}
		err2 = ioutil.WriteFile("./public.key.b64", []byte(pub64), 0644)
		if err2 != nil {
			logrus.Fatalln(err2)
		}
	}

	_, err = accountDB.CreateAccount(context.Background(), *username, pass, pub64, "")
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
