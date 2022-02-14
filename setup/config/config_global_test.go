package config

import (
	"testing"
)

func TestUserConsentOptions_Verify(t *testing.T) {
	type args struct {
		configErrors *ConfigErrors
		isMonolith   bool
	}
	tests := []struct {
		name    string
		fields  UserConsentOptions
		args    args
		wantErr bool
	}{
		{
			name: "template dir not set",
			fields: UserConsentOptions{
				RequireAtRegistration: true,
			},
			args: struct {
				configErrors *ConfigErrors
				isMonolith   bool
			}{configErrors: &ConfigErrors{}, isMonolith: true},
			wantErr: true,
		},
		{
			name: "template dir set",
			fields: UserConsentOptions{
				RequireAtRegistration: true,
				TemplateDir:           "testdata/privacy",
			},
			args: struct {
				configErrors *ConfigErrors
				isMonolith   bool
			}{configErrors: &ConfigErrors{}, isMonolith: true},
			wantErr: true,
		},
		{
			name: "policy name not set",
			fields: UserConsentOptions{
				RequireAtRegistration: true,
				TemplateDir:           "testdata/privacy",
			},
			args: struct {
				configErrors *ConfigErrors
				isMonolith   bool
			}{configErrors: &ConfigErrors{}, isMonolith: true},
			wantErr: true,
		},
		{
			name: "policy name set",
			fields: UserConsentOptions{
				RequireAtRegistration: true,
				TemplateDir:           "testdata/privacy",
				PolicyName:            "Privacy policy",
			},
			args: struct {
				configErrors *ConfigErrors
				isMonolith   bool
			}{configErrors: &ConfigErrors{}, isMonolith: true},
			wantErr: true,
		},
		{
			name: "version not set",
			fields: UserConsentOptions{
				RequireAtRegistration: true,
				TemplateDir:           "testdata/privacy",
			},
			args: struct {
				configErrors *ConfigErrors
				isMonolith   bool
			}{configErrors: &ConfigErrors{}, isMonolith: true},
			wantErr: true,
		},
		{
			name: "everyhing required set",
			fields: UserConsentOptions{
				RequireAtRegistration: true,
				TemplateDir:           "./testdata/privacy",
				Version:               "1.0",
				PolicyName:            "Privacy policy",
			},
			args: struct {
				configErrors *ConfigErrors
				isMonolith   bool
			}{configErrors: &ConfigErrors{}, isMonolith: true},
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := &UserConsentOptions{
				RequireAtRegistration:   tt.fields.RequireAtRegistration,
				PolicyName:              tt.fields.PolicyName,
				Version:                 tt.fields.Version,
				TemplateDir:             tt.fields.TemplateDir,
				SendServerNoticeToGuest: tt.fields.SendServerNoticeToGuest,
				ServerNoticeContent:     tt.fields.ServerNoticeContent,
				BlockEventsError:        tt.fields.BlockEventsError,
			}
			c.Verify(tt.args.configErrors, tt.args.isMonolith)
			if tt.wantErr && tt.args.configErrors == nil {
				t.Errorf("expected no errors, got '%+v'", tt.args.configErrors)
			}
		})
	}
}
