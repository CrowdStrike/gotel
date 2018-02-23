package gotel

import (
	"bytes"
	"flag"
	"fmt"
	"net/smtp"
	"strings"
	"time"
)

var (
	smtpHost string
	smtpUser string
	smtpPass string
	smtpPort int
)

func init() {
	flag.StringVar(&smtpHost, "GOTEL_SMTP_HOST", "", "Host of the SMTP server for sending mail")
	flag.StringVar(&smtpUser, "GOTEL_SMTP_USER", "", "SMTP user name")
	flag.StringVar(&smtpPass, "GOTEL_SMTP_PASS", "", "SMTP password")
	flag.IntVar(&smtpPort, "GOTEL_SMTP_PORT", 25, "SMTP port")
}

func (s *smtpAlerter) Bootstrap() {
	if smtpHost == "" {
		panic("You have SMTP alerting enabled but have not provided the GOTEL_SMTP_HOST env/flag")
	}
	l.info("alerter_smtp SMTP_HOST [%s] SMTP_USER [%s]", smtpHost, smtpUser)
}

type smtpAlerter struct {
	Cfg config
}

func (s *smtpAlerter) Name() string {
	return "SMTP"
}

func (s *smtpAlerter) Alert(res reservation) bool {

	ip, err := externalIP()
	if err != nil {
		ip = "N/A"
	}

	l.info("building SMTP alert for app [%s] component [%s] on ip [%s]\n", res.App, res.Component, ip)

	peopleToNotify := strings.Split(res.Notify, ",")

	// split on the notifiers and send a unique email to each.
	for _, emailAddyRaw := range peopleToNotify {

		smtpPair := fmt.Sprintf("%s:%d", smtpHost, smtpPort)
		emailAddy := strings.TrimSpace(emailAddyRaw)
		now := time.Now().Format(time.RFC822) // in case email delivery delay, let them know the actual date
		subject := "Job Failed to checkin"
		body := fmt.Sprintf("app [%s] component [%s] failed checkin on IP [%s]. Contact owner [%s]. Alert time is [%s]\n\nNotification list [%s]", res.App, res.Component, ip, res.Owner, now, res.Notify)
		message := bytes.NewBufferString(fmt.Sprintf("Subject: %s\r\nFrom: %s\r\nReply-to: %s\r\nTo: %s\r\n\r\n%s", subject, s.Cfg.Smtp.Fromaddress, s.Cfg.Smtp.ReplyTO, emailAddy, body))

		// Now push out the complete mail message
		auth := smtp.PlainAuth("", smtpUser, smtpPass, smtpHost)
		if err = smtp.SendMail(smtpPair, auth, s.Cfg.Smtp.Fromaddress, []string{emailAddy}, message.Bytes()); err != nil {
			l.warn("[WARN] Unable to write to mail server: host: [%s] user: [%s] err: [%v]\n", smtpHost, smtpUser, err)
			return false
		}
		l.info("Email sent for app [%s] component [%s]\n", res.App, res.Component)
	}

	return true
}
