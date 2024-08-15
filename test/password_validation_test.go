package main

import "testing"

func TestValidatePassword(t *testing.T) {
	tests := []struct {
		password string
		config   PasswordConfig
		expectErr bool
	}{
		// Test cases for length
		{"short", defaultPasswordConfig, true},
		{"longEnoughPassword1!", defaultPasswordConfig, false},
		{string(make([]byte, maxPasswordLength+1)), defaultPasswordConfig, true},
		
		// Test cases for character requirements
		{"NoDigitsOrSpecialChars", defaultPasswordConfig, true},
		{"WithDigits1", defaultPasswordConfig, true},
		{"WithSpecialChars!", defaultPasswordConfig, true},
		{"ValidP@ssw0rd", defaultPasswordConfig, false},

		// Custom config examples
		{"NoSpecialChar123", PasswordConfig{minPasswordLength, maxPasswordLength, true, true, true, false}, false},
		{"alllowercase1!", PasswordConfig{minPasswordLength, maxPasswordLength, false, true, true, true}, false},
	}

	for _, tt := range tests {
		err := ValidatePassword(tt.password, tt.config)
		if (err != nil) != tt.expectErr {
			t.Errorf("ValidatePassword(%s) = %v, expected error = %v", tt.password, err, tt.expectErr)
		}
	}
}
