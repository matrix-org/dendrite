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
	"crypto/ed25519"
	"encoding/base64"
	"flag"
	"fmt"
	"os"

	"github.com/sirupsen/logrus"
)

const usage = `Usage: %s

Sign a string using an Ed25519 private key

Arguments:

`

var (
	privateKeyFile = flag.String("private-key", "", "An Ed25519 private key seed used to sign the input")
	input          = flag.String("input", "", "The input to sign")
)

func main() {
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, usage, os.Args[0])
		flag.PrintDefaults()
	}

	flag.Parse()

	if *privateKeyFile == "" || *input == "" {
		flag.Usage()
		return
	}

	if *privateKeyFile != "" && *input != "" {
		seedBytes, err := os.ReadFile(*privateKeyFile)
		if err != nil {
			logrus.Fatalln(err)
		}
		priv := ed25519.NewKeyFromSeed(seedBytes)
		sig := ed25519.Sign(priv, []byte(*input))
		logrus.Infoln("Signature: " + base64.StdEncoding.EncodeToString(sig))
	}
}
