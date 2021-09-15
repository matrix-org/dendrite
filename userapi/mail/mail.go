package mail

import (
	"bytes"
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"strings"
	"sync"

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

// SmtpMailer is safe for concurrent use. It will block if email sending is in progress as long as it uses single connection.
type SmtpMailer struct {
	conf      config.EmailConf
	templates []*template.Template
	auth      smtp.Auth
	cl        *smtp.Client
	// sendMutex guards ensures that MAIL, RCPT and DATA commands are not messed between mails.
	sendMutex sync.Mutex
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
	if err = validateLine(mail.To); err != nil {
		return err
	}
	// lock at the point when data are prepared and we are executing commands
	m.sendMutex.Lock()
	defer m.sendMutex.Unlock()
	if err = m.cl.Mail(m.conf.From); err != nil {
		return err
	}
	if err = m.cl.Rcpt(mail.To); err != nil {
		return err
	}
	var w io.WriteCloser
	w, err = m.cl.Data()
	if err != nil {
		return err
	}
	_, err = w.Write(b.Bytes())
	if err != nil {
		return err
	}
	return w.Close()
}

func NewMailer(c *config.UserAPI) (Mailer, error) {
	if err := validateLine(c.Email.From); err != nil {
		return nil, err
	}
	sessionTypes := api.ThreepidSessionTypes()
	templates := make([]*template.Template, len(sessionTypes))
	for _, t := range sessionTypes {
		name := t.Name()
		templateRaw, err := ioutil.ReadFile(fmt.Sprintf("%s/%s.eml", c.Email.TemplatesPath, name))
		if err != nil {
			return nil, err
		}
		template, err := template.New(name).Parse(string(templateRaw))
		if err != nil {
			return nil, err
		}
		templates[t] = template
	}
	cl, err := smtp.Dial(c.Email.Smtp.Host)
	if err != nil {
		return nil, err
	}
	// defer c.Close() # TODO exit gracefully
	if err = cl.Hello("localhost"); err != nil {
		return nil, err
	}
	if ok, _ := cl.Extension("STARTTLS"); ok {
		config := &tls.Config{ServerName: c.Email.Smtp.Host}
		if err = cl.StartTLS(config); err != nil {
			return nil, err
		}
	}
	var auth smtp.Auth
	if c.Email.Smtp.User != "" {
		auth = smtp.PlainAuth(
			"",
			c.Email.Smtp.User,
			c.Email.Smtp.Password,
			c.Email.Smtp.Host,
		)
		if err = cl.Auth(auth); err != nil {
			return nil, err
		}

	}
	return &SmtpMailer{
		conf:      c.Email,
		templates: templates,
		auth:      auth,
		cl:        cl,
		sendMutex: sync.Mutex{},
	}, nil

}

// validateLine checks to see if a line has CR or LF as per RFC 5321
func validateLine(line string) error {
	if strings.ContainsAny(line, "\n\r") {
		return errors.New("smtp: A line must not contain CR or LF")
	}
	return nil
}
