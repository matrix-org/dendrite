package mail

import (
	"bytes"
	"fmt"
	"io/ioutil"
	"net/smtp"
	"text/template"
	"time"

	"github.com/matrix-org/dendrite/internal"
	"github.com/matrix-org/dendrite/setup/config"
	"github.com/matrix-org/dendrite/userapi/api"
)

const (
	messageIdByteLength = 48
)

type Mailer interface {
	// Send is used in
	// - https://matrix.org/docs/spec/client_server/r0.6.1#post-matrix-client-r0-account-3pid-email-requesttoken
	// - https://matrix.org/docs/spec/client_server/r0.6.1#post-matrix-client-r0-register-email-requesttoken
	// - https://matrix.org/docs/spec/client_server/r0.6.1#post-matrix-client-r0-account-password-email-requesttoken
	Send(*Mail, api.ThreepidSessionType) error
}
type SmtpMailer struct {
	conf      config.EmailConf
	templates map[api.ThreepidSessionType]*template.Template
}

type Mail struct {
	To    string
	Link  string
	Token string
	Extra []string
}

type Substitutions struct {
	*Mail
	Date      string
	MessageId string
}

func (m *SmtpMailer) Send(mail *Mail, t api.ThreepidSessionType) error {
	return m.send(mail, m.templates[t])
}

func (m *SmtpMailer) send(mail *Mail, t *template.Template) error {
	messageId, err := internal.GenerateBlob(messageIdByteLength)
	if err != nil {
		return err
	}
	s := Substitutions{
		Mail:      mail,
		Date:      time.Now().Format(time.RFC1123Z),
		MessageId: messageId,
	}
	b := bytes.Buffer{}
	err = t.Execute(&b, s)
	if err != nil {
		return err
	}
	return smtp.SendMail(
		m.conf.Smtp.Host,
		smtp.PlainAuth(
			"",
			m.conf.Smtp.User,
			m.conf.Smtp.Password,
			m.conf.Smtp.Host,
		),
		m.conf.From,
		[]string{
			mail.To,
		},
		b.Bytes(),
	)
}

func NewMailer(c *config.UserAPI) (Mailer, error) {
	templateRaw, err := ioutil.ReadFile(fmt.Sprintf("%s/verification.eml", c.Email.TemplatesPath))
	if err != nil {
		return nil, err
	}
	verificationT, err := template.New("verification").Parse(string(templateRaw))
	if err != nil {
		return nil, err
	}
	templateRaw, err = ioutil.ReadFile(fmt.Sprintf("%s/password.eml", c.Email.TemplatesPath))
	if err != nil {
		return nil, err
	}
	passwordT, err := template.New("password").Parse(string(templateRaw))
	return &SmtpMailer{
		conf: c.Email,
		templates: map[api.ThreepidSessionType]*template.Template{
			api.Password:     passwordT,
			api.Verification: verificationT,
		},
	}, err

}
