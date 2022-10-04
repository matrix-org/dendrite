package main

import (
	"encoding/json"
	"flag"
	"os"
	"reflect"
	"time"

	"github.com/invopop/jsonschema"
	"github.com/matrix-org/dendrite/setup/config"
	"github.com/uber/jaeger-client-go"
	"gopkg.in/yaml.v2"
)

func mapper(rt reflect.Type) *jsonschema.Schema {

	var d time.Duration
	if reflect.TypeOf(d) == rt {
		return &jsonschema.Schema{
			Type:        "string",
			Title:       "Duration",
			Description: "time.Duration with h, m, s to indicate hours, minutes, seconds",
			Pattern:     "^[0-9][0-9hms]*$",
		}
	}

	var du config.DataUnit
	if reflect.TypeOf(du) == rt {
		return &jsonschema.Schema{
			OneOf: []*jsonschema.Schema{
				&jsonschema.Schema{
					Type: "integer",
				},
				&jsonschema.Schema{
					Type:    "string",
					Pattern: "^[0-9]+([tgmk]b)?$",
				},
			},
			Title: "Data Unit", Description: "Data unit with suffix as tb, gb, mb, kb",
		}
	}

	// Cannot set this option in yaml config
	var js jaeger.SamplerOption
	if reflect.TypeOf(js) == rt {
		return &jsonschema.Schema{
			Type:  "string",
			Title: "Ignore this",
		}
	}
	return nil
}

func main() {
	useJson := false
	flag.BoolVar(&useJson, "json", useJson, "Output json instead of yaml")
	flag.Parse()

	reflector := jsonschema.Reflector{
		RequiredFromJSONSchemaTags: true,
		DoNotReference:             true,
		ExpandedStruct:             true,
		Mapper:                     mapper,
	}

	if err := reflector.AddGoComments("github.com/matrix-org/dendrite", "."); err != nil {
		panic(err)
	}

	schema := reflector.Reflect(config.Dendrite{})

	json, err := json.Marshal(schema)
	if err != nil {
		panic(err)
	}

	var data []byte
	if useJson {
		data = json
	} else {
		data, err = jsonToYaml(json)
		if err != nil {
			panic(err)
		}
	}

	if _, err := os.Stdout.Write(data); err != nil {
		panic(err)
	}
}

func jsonToYaml(json []byte) ([]byte, error) {
	var (
		a   any
		yml []byte
		err error
	)
	if err = yaml.Unmarshal(json, &a); err != nil {
		return nil, err
	}

	if yml, err = yaml.Marshal(a); err != nil {
		return nil, err
	}
	return yml, nil
}
