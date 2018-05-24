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

	yaml "gopkg.in/yaml.v2"
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
			config.Derived.ApplicationServices, appservice,
		)
	}

	// Check for any errors in the loaded application services
	return checkErrors(config)
}

// setupRegexps will create regex objects for exclusive and non-exclusive
// usernames, aliases and rooms of all application services, so that other
// methods can quickly check if a particular string matches any of them.
func setupRegexps(cfg *Dendrite) (err error) {
	// Combine all exclusive namespaces for later string checking
	var exclusiveUsernameStrings, exclusiveAliasStrings []string

	// If an application service's regex is marked as exclusive, add
	// its contents to the overall exlusive regex string. Room regex
	// not necessary as we aren't denying exclusive room ID creation
	for _, appservice := range cfg.Derived.ApplicationServices {
		for key, namespaceSlice := range appservice.NamespaceMap {
			switch key {
			case "users":
				appendExclusiveNamespaceRegexs(&exclusiveUsernameStrings, namespaceSlice)
			case "aliases":
				appendExclusiveNamespaceRegexs(&exclusiveAliasStrings, namespaceSlice)
			}
		}
	}

	// Join the regexes together into one big regex.
	// i.e. "app1.*", "app2.*" -> "(app1.*)|(app2.*)"
	// Later we can check if a username or alias matches any exclusive regex and
	// deny access if it isn't from an application service
	exclusiveUsernames := strings.Join(exclusiveUsernameStrings, "|")
	exclusiveAliases := strings.Join(exclusiveAliasStrings, "|")

	// If there are no exclusive regexes, compile string so that it will not match
	// any valid usernames/aliases/roomIDs
	if exclusiveUsernames == "" {
		exclusiveUsernames = "^$"
	}
	if exclusiveAliases == "" {
		exclusiveAliases = "^$"
	}

	// Store compiled Regex
	if cfg.Derived.ExclusiveApplicationServicesUsernameRegexp, err = regexp.Compile(exclusiveUsernames); err != nil {
		return err
	}
	if cfg.Derived.ExclusiveApplicationServicesAliasRegexp, err = regexp.Compile(exclusiveAliases); err != nil {
		return err
	}

	return nil
}

// appendExclusiveNamespaceRegexs takes a slice of strings and a slice of
// namespaces and will append the regexes of only the exclusive namespaces
// into the string slice
func appendExclusiveNamespaceRegexs(
	exclusiveStrings *[]string, namespaces []ApplicationServiceNamespace,
) {
	for index, namespace := range namespaces {
		if namespace.Exclusive {
			// We append parenthesis to later separate each regex when we compile
			// i.e. "app1.*", "app2.*" -> "(app1.*)|(app2.*)"
			*exclusiveStrings = append(*exclusiveStrings, "("+namespace.Regex+")")
		}

		// Compile this regex into a Regexp object for later use
		namespaces[index].RegexpObject, _ = regexp.Compile(namespace.Regex)
	}
}

// checkErrors checks for any configuration errors amongst the loaded
// application services according to the application service spec.
func checkErrors(config *Dendrite) (err error) {
	var idMap = make(map[string]bool)
	var tokenMap = make(map[string]bool)

	// Check each application service for any config errors
	for _, appservice := range config.Derived.ApplicationServices {
		// Check if we've already seen this ID. No two application services
		// can have the same ID or token.
		if idMap[appservice.ID] {
			return configErrors([]string{fmt.Sprintf(
				"Application service ID %s must be unique", appservice.ID,
			)})
		}
		// Check if we've already seen this token
		if tokenMap[appservice.ASToken] {
			return configErrors([]string{fmt.Sprintf(
				"Application service Token %s must be unique", appservice.ASToken,
			)})
		}

		// Add the id/token to their respective maps if we haven't already
		// seen them.
		idMap[appservice.ID] = true
		tokenMap[appservice.ASToken] = true

		// Check if more than one regex exists per namespace
		for _, namespace := range appservice.NamespaceMap {
			if len(namespace) > 1 {
				// It's quite easy to accidentally make multiple regex objects per
				// namespace, which often ends up in an application service receiving events
				// it doesn't want, as an empty regex will match all events.
				return configErrors([]string{fmt.Sprintf(
					"Application service namespace can only contain a single regex tuple. Check your YAML.",
				)})
			}
		}
	}

	// Check that namespace(s) are valid regex
	for _, appservice := range config.Derived.ApplicationServices {
		for _, namespaceSlice := range appservice.NamespaceMap {
			for _, namespace := range namespaceSlice {
				if !IsValidRegex(namespace.Regex) {
					return configErrors([]string{fmt.Sprintf(
						"Invalid regex string for application service %s", appservice.ID,
					)})
				}
			}
		}
	}

	return setupRegexps(config)
}

// IsValidRegex returns true or false based on whether the
// given string is valid regex or not
func IsValidRegex(regexString string) bool {
	_, err := regexp.Compile(regexString)

	return err == nil
}
