package main

import (
	"bytes"
	"io"
	"testing"
)

func Test_getPassword(t *testing.T) {
	type args struct {
		password string
		pwdFile  string
		pwdStdin bool
		reader   io.Reader
	}

	pass := "mySecretPass"
	passwordFile := "testdata/my.pass"
	reader := &bytes.Buffer{}
	_, err := reader.WriteString(pass)
	if err != nil {
		t.Errorf("unable to write to buffer: %+v", err)
	}
	tests := []struct {
		name    string
		args    args
		want    string
		wantErr bool
	}{
		{
			name: "password defined",
			args: args{
				password: pass,
			},
			want: pass,
		},
		{
			name: "pwdFile defined",
			args: args{
				pwdFile: passwordFile,
			},
			want: pass,
		},
		{
			name:    "pwdFile does not exist",
			args:    args{pwdFile: "iDontExist"},
			wantErr: true,
		},
		{
			name: "read pass from stdin defined",
			args: args{
				pwdStdin: true,
				reader:   reader,
			},
			want: pass,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := getPassword(tt.args.password, tt.args.pwdFile, tt.args.pwdStdin, tt.args.reader)
			if !tt.wantErr && err != nil {
				t.Errorf("expected no error, but got %v", err)
			}
			if got != tt.want {
				t.Errorf("getPassword() = '%v', want '%v'", got, tt.want)
			}
		})
	}
}
