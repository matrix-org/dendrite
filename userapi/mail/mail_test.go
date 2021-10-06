//go:build test_mail
// +build test_mail

package mail

import (
	"testing"

	"github.com/matrix-org/dendrite/setup/config"
	"github.com/matrix-org/dendrite/userapi/api"
	"github.com/matryer/is"
)

func TestSendVerification(t *testing.T) {
	is := is.New(t)
	dut := mustNewMailer(is)
	err := dut.Send(
		&Mail{
			To:    "kevil11378@186site.com",
			Link:  "http://my",
			Token: "foo",
			Extra: []string{
				"bar",
			},
		}, api.Register)
	is.NoErr(err)
}

func mustNewMailer(is *is.I) Mailer {
	mailer, err := NewMailer(&config.UserAPI{
		Email: config.EmailConf{
			TemplatesPath: "../../res/default",
			From:          "test@matrix.com",
			Smtp: config.Smtp{
				Host: "localhost:25",
			},
		},
	})
	is.NoErr(err)
	return mailer
}
