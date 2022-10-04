package main

import (
	"flag"
	"testing"

	"github.com/matrix-org/dendrite/setup/config"
)

func TestBuildConfig(t *testing.T) {
	tsts := []struct {
		Name       string
		Args       []string
		IsMonolith bool
	}{
		{"ciMonolith", []string{"-ci"}, true},
		{"ciPolylith", []string{"-ci", "-polylith"}, false},
	}
	for _, tst := range tsts {
		t.Run(tst.Name, func(t *testing.T) {
			cfg, err := buildConfig(flag.NewFlagSet("main_test", flag.ContinueOnError), tst.Args)
			if err != nil {
				t.Fatalf("buildConfig failed: %v", err)
			}

			var ss config.ConfigErrors
			cfg.Verify(&ss, tst.IsMonolith)
			for _, s := range ss {
				t.Errorf("Verify: %s", s)
			}
		})
	}
}
