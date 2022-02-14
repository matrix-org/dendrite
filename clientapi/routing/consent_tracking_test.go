package routing

import "testing"

func Test_validHMAC(t *testing.T) {
	type args struct {
		username string
		userHMAC string
		secret   string
	}
	tests := []struct {
		name    string
		args    args
		want    bool
		wantErr bool
	}{
		{
			name:    "invalid hmac",
			args:    args{},
			wantErr: false,
			want:    false,
		},
		// $ echo -n '@alice:localhost' | openssl sha256 -hmac 'helloWorld'                                              27m ⚑ ◒  15:35:54
		//(stdin)= 121c9bab767ed87a3136db0c3002144dfe414720aa328d235199082e4757541e
		//
		{
			name: "valid hmac",
			args: args{
				username: "@alice:localhost",
				userHMAC: "121c9bab767ed87a3136db0c3002144dfe414720aa328d235199082e4757541e",
				secret:   "helloWorld",
			},
			want: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := validHMAC(tt.args.username, tt.args.userHMAC, tt.args.secret)
			if (err != nil) != tt.wantErr {
				t.Errorf("validHMAC() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if got != tt.want {
				t.Errorf("validHMAC() got = %v, want %v", got, tt.want)
			}
		})
	}
}
