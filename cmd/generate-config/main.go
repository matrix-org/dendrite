package main

import (
	"fmt"

	"github.com/matrix-org/dendrite/internal/config"
	"gopkg.in/yaml.v2"
)

func main() {
	cfg := &config.Dendrite{}
	cfg.Defaults()
	cfg.Logging = []config.LogrusHook{
		{
			Type:  "file",
			Level: "info",
			Params: map[string]interface{}{
				"path": "/var/log/dendrite",
			},
		},
	}

	j, err := yaml.Marshal(cfg)
	if err != nil {
		panic(err)
	}

	fmt.Println(string(j))
}
