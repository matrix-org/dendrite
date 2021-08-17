package config

import (
	"testing"
)

func TestPasswordRequirementsDefaults(t *testing.T) {
	requirements := &PasswordRequirements{}
	requirements.Defaults()

	if requirements.MinPasswordLength <= 0 {
		t.Errorf("default minimum password length should be greater than 0, got %d", requirements.MinPasswordLength)
	}

	if requirements.MinPasswordLength > requirements.MaxPasswordLength {
		t.Errorf(
			"default minimum password length should not be greater than maxmium password length. currently: %d > %d",
			requirements.MinPasswordLength,
			requirements.MaxPasswordLength,
		)
	}
}
