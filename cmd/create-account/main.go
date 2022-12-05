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
	"bytes"
	"crypto/hmac"
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"regexp"
	"strings"
	"time"

	"github.com/tidwall/gjson"

	"github.com/sirupsen/logrus"
	"golang.org/x/term"

	"github.com/matrix-org/dendrite/setup"
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

Arguments:

`

var (
	username           = flag.String("username", "", "The username of the account to register (specify the localpart only, e.g. 'alice' for '@alice:domain.com')")
	password           = flag.String("password", "", "The password to associate with the account")
	pwdFile            = flag.String("passwordfile", "", "The file to use for the password (e.g. for automated account creation)")
	pwdStdin           = flag.Bool("passwordstdin", false, "Reads the password from stdin")
	isAdmin            = flag.Bool("admin", false, "Create an admin account")
	resetPassword      = flag.Bool("reset-password", false, "Deprecated")
	serverURL          = flag.String("url", "http://localhost:8008", "The URL to connect to.")
	validUsernameRegex = regexp.MustCompile(`^[0-9a-z_\-=./]+$`)
	timeout            = flag.Duration("timeout", time.Second*30, "Timeout for the http client when connecting to the server")
)

var cl = http.Client{
	Timeout:   time.Second * 30,
	Transport: http.DefaultTransport,
}

func main() {
	name := os.Args[0]
	flag.Usage = func() {
		_, _ = fmt.Fprintf(os.Stderr, usage, name, name, name, name, name, name)
		flag.PrintDefaults()
	}
	cfg := setup.ParseFlags(true)

	if *resetPassword {
		logrus.Fatalf("The reset-password flag has been replaced by the POST /_dendrite/admin/resetPassword/{localpart} admin API.")
	}

	if cfg.ClientAPI.RegistrationSharedSecret == "" {
		logrus.Fatalln("Shared secret registration is not enabled, enable it by setting a shared secret in the config: 'client_api.registration_shared_secret'")
	}

	if *username == "" {
		flag.Usage()
		os.Exit(1)
	}

	if !validUsernameRegex.MatchString(*username) {
		logrus.Warn("Username can only contain characters a-z, 0-9, or '_-./='")
		os.Exit(1)
	}

	if len(fmt.Sprintf("@%s:%s", *username, cfg.Global.ServerName)) > 255 {
		logrus.Fatalf("Username can not be longer than 255 characters: %s", fmt.Sprintf("@%s:%s", *username, cfg.Global.ServerName))
	}

	pass, err := getPassword(*password, *pwdFile, *pwdStdin, os.Stdin)
	if err != nil {
		logrus.Fatalln(err)
	}

	cl.Timeout = *timeout

	accessToken, err := sharedSecretRegister(cfg.ClientAPI.RegistrationSharedSecret, *serverURL, *username, pass, *isAdmin)
	if err != nil {
		logrus.Fatalln("Failed to create the account:", err.Error())
	}

	logrus.Infof("Created account: %s (AccessToken: %s)", *username, accessToken)
}

type sharedSecretRegistrationRequest struct {
	User     string `json:"username"`
	Password string `json:"password"`
	Nonce    string `json:"nonce"`
	MacStr   string `json:"mac"`
	Admin    bool   `json:"admin"`
}

func sharedSecretRegister(sharedSecret, serverURL, localpart, password string, admin bool) (accessToken string, err error) {
	registerURL := fmt.Sprintf("%s/_synapse/admin/v1/register", strings.Trim(serverURL, "/"))
	nonceReq, err := http.NewRequest(http.MethodGet, registerURL, nil)
	if err != nil {
		return "", fmt.Errorf("unable to create http request: %w", err)
	}
	nonceResp, err := cl.Do(nonceReq)
	if err != nil {
		return "", fmt.Errorf("unable to get nonce: %w", err)
	}
	body, err := io.ReadAll(nonceResp.Body)
	if err != nil {
		return "", fmt.Errorf("failed to read response body: %w", err)
	}
	defer nonceResp.Body.Close() // nolint: errcheck

	nonce := gjson.GetBytes(body, "nonce").Str

	adminStr := "notadmin"
	if admin {
		adminStr = "admin"
	}
	reg := sharedSecretRegistrationRequest{
		User:     localpart,
		Password: password,
		Nonce:    nonce,
		Admin:    admin,
	}
	macStr, err := getRegisterMac(sharedSecret, nonce, localpart, password, adminStr)
	if err != nil {
		return "", err
	}
	reg.MacStr = macStr

	js, err := json.Marshal(reg)
	if err != nil {
		return "", fmt.Errorf("unable to marshal json: %w", err)
	}
	registerReq, err := http.NewRequest(http.MethodPost, registerURL, bytes.NewBuffer(js))
	if err != nil {
		return "", fmt.Errorf("unable to create http request: %w", err)

	}
	regResp, err := cl.Do(registerReq)
	if err != nil {
		return "", fmt.Errorf("unable to create account: %w", err)
	}
	defer regResp.Body.Close() // nolint: errcheck
	if regResp.StatusCode < 200 || regResp.StatusCode >= 300 {
		body, _ = io.ReadAll(regResp.Body)
		return "", fmt.Errorf("got HTTP %d error from server: %s", regResp.StatusCode, string(body))
	}
	r, err := io.ReadAll(regResp.Body)
	if err != nil {
		return "", fmt.Errorf("failed to read response body (HTTP %d): %w", regResp.StatusCode, err)
	}

	return gjson.GetBytes(r, "access_token").Str, nil
}

func getRegisterMac(sharedSecret, nonce, localpart, password, adminStr string) (string, error) {
	joined := strings.Join([]string{nonce, localpart, password, adminStr}, "\x00")
	mac := hmac.New(sha1.New, []byte(sharedSecret))
	_, err := mac.Write([]byte(joined))
	if err != nil {
		return "", fmt.Errorf("unable to construct mac: %w", err)
	}
	regMac := mac.Sum(nil)

	return hex.EncodeToString(regMac), nil
}

func getPassword(password, pwdFile string, pwdStdin bool, r io.Reader) (string, error) {
	// read password from file
	if pwdFile != "" {
		pw, err := os.ReadFile(pwdFile)
		if err != nil {
			return "", fmt.Errorf("Unable to read password from file: %v", err)
		}
		return strings.TrimSpace(string(pw)), nil
	}

	// read password from stdin
	if pwdStdin {
		data, err := io.ReadAll(r)
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
