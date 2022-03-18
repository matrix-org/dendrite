package main

import (
	"bytes"
	"io"
	"testing"
)

func Test_getPassword(t *testing.T) {
	type args struct {
		password *string
		pwdFile  *string
		pwdStdin *bool
		reader   io.Reader
		writer   func()
	}

	pass := "mySecretPass"
	empty := ""
	f := false
	passwordFile := "testdata/my.pass"
	passwordStdin := true
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
				password: &pass,
				pwdFile:  &empty,
				pwdStdin: &f,
			},
			want: pass,
		},
		{
			name: "pwdFile defined",
			args: args{
				pwdFile:  &passwordFile,
				password: &empty,
				pwdStdin: &f,
			},
			want: pass,
		},
		{
			name: "read pass from stdin defined",
			args: args{
				pwdStdin: &passwordStdin,
				reader:   reader,
				password: &empty,
				pwdFile:  &empty,
			},
			want: pass,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.args.writer != nil {
				go tt.args.writer()
			}
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
