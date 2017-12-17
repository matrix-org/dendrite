// Copyright 2017 Andrew Morgan <andrew@amorgan.xyz>
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

package config

import (
	"io/ioutil"
	"path/filepath"

	"gopkg.in/yaml.v2"
)

// ApplicationServiceNamespace is the namespace that a specific application
// service has management over.
type ApplicationServiceNamespace struct {
	// Whether or not the namespace is managed solely by this application service
	Exclusive bool `yaml:"exclusive"`
	// A regex pattern that represents the namespace
	Regex string `yaml:"regex"`
}

// ApplicationService represents a Matrix application service.
// https://matrix.org/docs/spec/application_service/unstable.html
type ApplicationService struct {
	// User-defined, unique, persistent ID of the application service
	ID string `yaml:"id"`
	// Base URL of the application service
	URL string `yaml:"url"`
	// Application service token provided in requests to a homeserver
	ASToken string `yaml:"as_token"`
	// Homeserver token provided in requests to an application service
	HSToken string `yaml:"hs_token"`
	// Localpart of application service user
	SenderLocalpart string `yaml:"sender_localpart"`
	// Information about an application service's namespaces
	Namespaces map[string][]interface{} `yaml:"namespaces"`
}

func loadAppservices(config *Dendrite) error {
	// Iterate through and return all the Application Services
	for _, configPath := range config.ApplicationService.ConfigFiles {
		// Create a new application service
		var appservice ApplicationService

		// Create an absolute path from a potentially relative path
		absPath, err := filepath.Abs(configPath)
		if err != nil {
			return err
		}

		// Read the application service's config file
		configData, err := ioutil.ReadFile(absPath)
		if err != nil {
			return err
		}

		// Load the config data into our struct
		if err = yaml.UnmarshalStrict(configData, &appservice); err != nil {
			return err
		}

		// Append the parsed application service to the global config
		config.Derived.ApplicationServices = append(
			config.Derived.ApplicationServices, appservice)
	}

	return nil
}
