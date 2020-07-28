package main

import (
	"fmt"

	"github.com/matrix-org/dendrite/internal/config"
	"gopkg.in/yaml.v2"
)

func main() {
	config := &config.Dendrite{}
	config.Defaults()

	j, err := yaml.Marshal(config)
	if err != nil {
		panic(err)
	}

	fmt.Println(string(j))
}
