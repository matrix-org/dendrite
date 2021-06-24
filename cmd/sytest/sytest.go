package main

import (
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net"
	"net/smtp"
	"os"
	"os/exec"

	"github.com/sirupsen/logrus"
)

func main() {
	err := exec.Command("git", "pull").Run()
	if err != nil {
		logrus.WithError(err).Fatal("Run git pull failed")
	}
	logrus.Info("Git update done")
	err = os.RemoveAll("./cmd/sytest/result")
	if err != nil && !os.IsNotExist(err) {
		logrus.WithError(err).Fatal("Remove old result failed")
	}
	file, err := ioutil.ReadFile("./cmd/sytest/config.json")
	if err != nil {
		logrus.WithError(err).Fatal("Read config file failed")
	}
	var cfg struct {
		Src      string `json:"src"`
		SendMail bool   `json:"send_mail"`
		Username string `json:"username"`
		Password string `json:"password"`
		Host     string `json:"host"`
		Port     string `json:"port"`
	}
	err = json.Unmarshal(file, &cfg)
	if err != nil {
		logrus.WithError(err).Fatal("Unmarshal config file failed")
	}
	res, err := exec.Command("docker", "run", "--rm",
		"-v", cfg.Src+":/src/",
		"-v", cfg.Src+"cmd/sytest/result:/logs/",
		"matrixdotorg/sytest-dendrite").Output()
	if err != nil {
		logrus.WithField("output", string(res)).WithError(err).Fatal("Run sytest docker image failed")
	}
	logrus.Info("Sytest done")
	out, err := exec.Command("./are-we-synapse-yet.py",
		"-v", "./cmd/sytest/result/results.tap").Output()
	if err != nil {
		logrus.WithError(err).Fatal("Run are-we-synapse-yet failed")
	}
	if cfg.SendMail {
		auth := smtp.PlainAuth("",
			cfg.Username,
			cfg.Password,
			cfg.Host)
		to := []string{"all@workly.ai"}
		content := []byte(fmt.Sprintf("From:%s\r\nTo:all@workly.ai\r\nSubject:Are We Synapse Yet?\r\nContent-Type:text/plain;charset=utf-8\r\n\r\n%s", cfg.Username, out))
		err = sendMail(cfg.Host+":"+cfg.Port, auth, cfg.Username, to, content)
		if err != nil {
			logrus.WithError(err).Fatal("Send mail failed")
		}
	} else {
		logrus.Info("\n" + string(out))
	}
}

func sendMail(addr string, auth smtp.Auth, from string, to []string, msg []byte) (err error) {
	c, err := dial(addr)
	if err != nil {
		return err
	}
	if auth != nil {
		if ok, _ := c.Extension("AUTH"); ok {
			if err = c.Auth(auth); err != nil {
				return err
			}
		}
	}
	if err = c.Mail(from); err != nil {
		return err
	}
	for _, addr := range to {
		if err = c.Rcpt(addr); err != nil {
			fmt.Print(err)
			return err
		}
	}
	w, err := c.Data()
	if err != nil {
		return err
	}
	_, err = w.Write(msg)
	if err != nil {
		return err
	}
	err = w.Close()
	if err != nil {
		return err
	}
	return c.Quit()
}

func dial(addr string) (*smtp.Client, error) {
	conn, err := tls.Dial("tcp", addr, nil)
	if err != nil {
		return nil, err
	}
	host, _, _ := net.SplitHostPort(addr)
	return smtp.NewClient(conn, host)
}
