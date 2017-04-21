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
	"github.com/matrix-org/dendrite/clientapi/clientapi-app"
	"github.com/matrix-org/dendrite/common"

	log "github.com/Sirupsen/logrus"
	"github.com/docopt/docopt-go"
)

func maybeArgToStr(arg interface{}) string {
	if arg != nil {
		return arg.(string)
	}
	return ""
}

func main() {
	usage := `
Usage:
    dendrite serve all [options]
    dendrite serve <server-type> [options]
    dendrite -V
    dendrite -h

Arguments:
    <server-type>   One of: client-api, room-server, sync-api.

Options:
    -a <address>, --address=<address>
        Address to bind. The port is required but ignored for 'serve all'. The
        default ports are: 7776 (sync-api), 7777 (room-server), and 7778 (client-api)
        [default: localhost]
    -c <config-file>, --config-file=<config-file>
        Path to a YAML-format configuration file.
    -h, --help
        Print this usage text.
    -k <kafka-addresses>, --kafka-addresses=<kafka-addresses>
        A comma-separated list of Kafka addresses. [default: localhost:9092]
    -l <log-dir>, --log-dir=<log-dir>
        Path to log directory. If not set, logs will only be written to stderr.
    -r <room-server-host>, --room-server-host=<room-server-host>
        Host of the room server. [default: localhost:7777]
    -t <topic-prefix>, --topic-prefix=<topic-prefix>
        Prefix for Kafka topics used for inter-component communication.
    -V, --version
        Print the dendrite version.

Environment Variables:
    PostgreSQL MUST be configured using the standard libpq environment variables:
        https://www.postgresql.org/docs/current/static/libpq-envars.html
    Required:
        PGPASSWORD
        PGHOST
    Optional:
        PGUSER      Defaults to "postgres"
        PGSSLMODE   Defaults to "disabled"
`

	args, _ := docopt.Parse(usage, nil, true,
		"dendrite Matrix homeserver, version 0.0.1", false)

	logDir := maybeArgToStr(args["--log-dir"])
	common.SetupLogging(logDir)

	log.Infoln(args)

	if args["serve"] != nil && args["serve"].(bool) {
		if args["all"] != nil && args["all"].(bool) {
			log.Panic("'serve all' is not yet supported.")
		} else {
			switch serverType := args["<server-type>"]; serverType {
			case "client-api":
				log.Infof("Starting %v server...", serverType)
				address := maybeArgToStr(args["--address"])
				if address == "localhost" {
					address += ":7778"
				}
				clientapi.App(
					address,
					maybeArgToStr(args["--kafka-addresses"]),
					maybeArgToStr(args["--room-server-host"]),
					maybeArgToStr(args["--topic-prefix"]),
				)
			default:
				log.Panicf("Server type '%v' unknown.", serverType)
			}
		}
	}
}
