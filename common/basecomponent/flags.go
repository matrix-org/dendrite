// Copyright 2017 New Vector Ltd
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

package basecomponent

import (
	"flag"
	"os"

	"github.com/matrix-org/dendrite/common/config"

	"github.com/sirupsen/logrus"
)

var configPath = flag.String("config", EnvParse("DENDRITE_CONFIG_FILE", "dendrite.yaml"), "The path to the config file. For more information, see the config file in this repository.")

// ParseFlags parses the commandline flags and uses them to create a config.
// If running as a monolith use `ParseMonolithFlags` instead.
func ParseFlags() *config.Dendrite {
	flag.Parse()

	if *configPath == "" {
		logrus.Fatal("--config must be supplied")
	}

	cfg, err := config.Load(*configPath)

	if err != nil {
		logrus.Fatalf("Invalid config file: %s", err)
	}

	return cfg
}

// ParseMonolithFlags parses the commandline flags and uses them to create a
// config. Should only be used if running a monolith. See `ParseFlags`.
func ParseMonolithFlags() *config.Dendrite {
	flag.Parse()

	if *configPath == "" {
		logrus.Fatal("--config must be supplied")
	}

	cfg, err := config.LoadMonolithic(*configPath)

	if err != nil {
		logrus.Fatalf("Invalid config file: %s", err)
	}

	return cfg
}

// EnvParse returns the value of the environmentvariable if it exists, otherwise `def`.
func EnvParse(env string, def string) string {
	cnf, exists := os.LookupEnv(env)
	if !exists {
		cnf = def
	}
	return cnf
}
