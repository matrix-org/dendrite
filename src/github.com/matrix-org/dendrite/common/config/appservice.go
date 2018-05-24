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
	"fmt"
	"io/ioutil"
	"path/filepath"
	"regexp"
	"strings"

	"gopkg.in/yaml.v2"
)

// ApplicationServiceNamespace is the namespace that a specific application
// service has management over.
type ApplicationServiceNamespace struct {
	// Whether or not the namespace is managed solely by this application service
	Exclusive bool `yaml:"exclusive"`
	// A regex pattern that represents the namespace
	Regex string `yaml:"regex"`
	// Regex object representing our pattern. Saves having to recompile every time
	RegexpObject *regexp.Regexp
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
	// Information about an application service's namespaces. Key is either
	// "users", "aliases" or "rooms"
	NamespaceMap map[string][]ApplicationServiceNamespace `yaml:"namespaces"`
}

// loadAppservices iterates through all application service config files
// and loads their data into the config object for later access.
func loadAppservices(config *Dendrite) error {
	for _, configPath := range config.ApplicationServices.ConfigFiles {
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

	// Check for any errors in the loaded application services
	return checkErrors(config)
}

// setupRegexps will create regex objects for exclusive and non-exclusive
// usernames, aliases and rooms of all application services, so that other
// methods can quickly check if a particular string matches any of them.
func setupRegexps(cfg *Dendrite) {
	// Combine all exclusive namespaces for later string checking
	var exclusiveUsernameStrings, exclusiveAliasStrings, exclusiveRoomStrings []string

	// If an application service's regex is marked as exclusive, add
	// it's contents to the overall exlusive regex string
	for _, appservice := range cfg.Derived.ApplicationServices {
		for key, namespaceSlice := range appservice.NamespaceMap {
			switch key {
			case "users":
				appendExclusiveNamespaceRegexs(&exclusiveUsernameStrings, namespaceSlice)
			case "aliases":
				appendExclusiveNamespaceRegexs(&exclusiveAliasStrings, namespaceSlice)
			case "rooms":
				appendExclusiveNamespaceRegexs(&exclusiveRoomStrings, namespaceSlice)
			}
		}
	}

	// Join the regexes together into one big regex.
	// i.e. "app1.*", "app2.*" -> "(app1.*)|(app2.*)"
	// Later we can check if a username or some other string matches any exclusive
	// regex and deny access if it isn't from an application service
	exclusiveUsernames := strings.Join(exclusiveUsernameStrings, "|")
	exclusiveAliases := strings.Join(exclusiveAliasStrings, "|")
	exclusiveRooms := strings.Join(exclusiveRoomStrings, "|")

	// If there are no exclusive regexes, compile string so that it will not match
	// any valid usernames/aliases/roomIDs
	if exclusiveUsernames == "" {
		exclusiveUsernames = "^$"
	}
	if exclusiveAliases == "" {
		exclusiveAliases = "^$"
	}
	if exclusiveRooms == "" {
		exclusiveRooms = "^$"
	}

	// Store compiled Regex
	cfg.Derived.ExclusiveApplicationServicesUsernameRegexp, _ = regexp.Compile(exclusiveUsernames)
	cfg.Derived.ExclusiveApplicationServicesUsernameRegexp, _ = regexp.Compile(exclusiveAliases)
	cfg.Derived.ExclusiveApplicationServicesUsernameRegexp, _ = regexp.Compile(exclusiveRooms)
}

// appendExclusiveNamespaceRegexs takes a slice of strings and a slice of
// namespaces and will append the regexes of only the exclusive namespaces
// into the string slice
func appendExclusiveNamespaceRegexs(
	exclusiveStrings *[]string, namespaces []ApplicationServiceNamespace,
) {
	for _, namespace := range namespaces {
		if namespace.Exclusive {
			// We append parenthesis to later separate each regex when we compile
			// i.e. "app1.*", "app2.*" -> "(app1.*)|(app2.*)"
			*exclusiveStrings = append(*exclusiveStrings, "("+namespace.Regex+")")
		}

		// Compile this regex into a Regexp object for later use
		namespace.RegexpObject, _ = regexp.Compile(namespace.Regex)
	}
}

// checkErrors checks for any configuration errors amongst the loaded
// application services according to the application service spec.
func checkErrors(config *Dendrite) error {
	var idMap = make(map[string]bool)
	var tokenMap = make(map[string]bool)

	// Check that no two application services have the same as_token or id
	for _, appservice := range config.Derived.ApplicationServices {
		// Check if we've already seen this ID
		if idMap[appservice.ID] {
			return configErrors([]string{fmt.Sprintf(
				"Application Service ID %s must be unique", appservice.ID,
			)})
		}
		if tokenMap[appservice.ASToken] {
			return configErrors([]string{fmt.Sprintf(
				"Application Service Token %s must be unique", appservice.ASToken,
			)})
		}

		// Add the id/token to their respective maps if we haven't already
		// seen them.
		idMap[appservice.ID] = true
		tokenMap[appservice.ID] = true
	}

	// Check that namespace(s) are valid regex
	for _, appservice := range config.Derived.ApplicationServices {
		for _, namespaceSlice := range appservice.NamespaceMap {
			for _, namespace := range namespaceSlice {
				if !IsValidRegex(namespace.Regex) {
					return configErrors([]string{fmt.Sprintf(
						"Invalid regex string for Application Service %s", appservice.ID,
					)})
				}
			}
		}
	}
	setupRegexps(config)

	return nil
}

// IsValidRegex returns true or false based on whether the
// given string is valid regex or not
func IsValidRegex(regexString string) bool {
	_, err := regexp.Compile(regexString)

	return err == nil
}
