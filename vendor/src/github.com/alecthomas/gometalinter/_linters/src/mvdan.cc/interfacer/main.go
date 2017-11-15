// Copyright (c) 2015, Daniel Martí <mvdan@mvdan.cc>
// See LICENSE for licensing information

package main // import "mvdan.cc/interfacer"

import (
	"flag"
	"fmt"
	"os"

	"mvdan.cc/interfacer/check"
)

var _ = flag.Bool("v", false, "print the names of packages as they are checked")

func main() {
	flag.Parse()
	lines, err := check.CheckArgs(flag.Args())
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	for _, line := range lines {
		fmt.Println(line)
	}
}
