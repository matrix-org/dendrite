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
		askPass  *bool
		reader   io.Reader
	}

	pass := "mySecretPass"
	passwordFile := "testdata/my.pass"
	passwordStdin := true
	reader := &bytes.Buffer{}
	_, err := reader.WriteString(pass)
	if err != nil {
		t.Errorf("unable to write to buffer: %+v", err)
	}
	tests := []struct {
		name string
		args args
		want string
	}{
		{
			name: "no password defined",
			args: args{},
			want: "",
		},
		{
			name: "password defined",
			args: args{password: &pass},
			want: pass,
		},
		{
			name: "pwdFile defined",
			args: args{pwdFile: &passwordFile},
			want: pass,
		},
		{
			name: "read pass from stdin defined",
			args: args{
				pwdStdin: &passwordStdin,
				reader:   reader,
			},
			want: pass,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := getPassword(tt.args.password, tt.args.pwdFile, tt.args.pwdStdin, tt.args.askPass, tt.args.reader); got != tt.want {
				t.Errorf("getPassword() = '%v', want '%v'", got, tt.want)
			}
		})
	}
}
