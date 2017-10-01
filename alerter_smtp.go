package main

import (
	"bytes"
	"flag"
	"fmt"
	"net/smtp"
	"strings"
	"time"
)

var smtpHost string

func init() {
	flag.StringVar(&smtpHost, "GOTEL_SMTP_HOST", "", "Host of the SMTP server for sending mail")
}

func (s *smtpAlerter) Bootstrap() {
	if smtpHost == "" {
		panic("You have SMTP alerting enalbed but have not provided the GOTEL_SMTP_HOST env/flag")
	}
	l.info("alerter_smtp SMTP_HOST [%s]", smtpHost)
}

type smtpAlerter struct {
	Cfg config
}

func (s *smtpAlerter) Name() string {
	return "SMTP"
}

func (s *smtpAlerter) Alert(res reservation) {

	ip, err := externalIP()
	if err != nil {
		ip = "N/A"
	}

	l.info("SMTP alert on app [%s] component [%s] on ip [%s]\n", res.App, res.Component, ip)

	peopleToNotify := strings.Split(res.Notify, ",")

	// split on the notifiers and send a unique email to each.
	for _, emailAddyRaw := range peopleToNotify {

		emailAddy := strings.TrimSpace(emailAddyRaw)

		// Connect to the remote SMTP server.
		c, err := smtp.Dial(smtpHost + ":25")
		if err != nil {
			l.warn("[WARN] Unable to dial mail server: [%s] err: [%v]", smtpHost, err)
			return
		}
		// Set the sender in header.
		c.Mail(s.Cfg.Smtp.Fromaddress)
		// Set the recipient in header
		c.Rcpt(emailAddy)

		// Connecting to mail server ?.
		wc, err := c.Data()
		if err != nil {
			l.warn("[WARN] Unable to connect to mail server: [%s] err: [%v]", smtpHost, err)
			return
		}
		defer wc.Close()
		now := time.Now().Format(time.RFC822) // in case email delivery delay, let them know the actual date
		subject := "Job Failed to checkin"
		body := fmt.Sprintf("app [%s] component [%s] failed checkin on IP [%s]. Contact owner [%s]. Alert time is [%s]\n\nNotification list [%s]", res.App, res.Component, ip, res.Owner, now, res.Notify)
		message := bytes.NewBufferString(fmt.Sprintf("Subject: %s\r\nFrom: %s\r\nReply-to: %s\r\nTo: %s\r\n\r\n%s", subject, s.Cfg.Smtp.Fromaddress, s.Cfg.Smtp.ReplyTO, emailAddy, body))
		// Now push out the complete mail message

		if _, err = message.WriteTo(wc); err != nil {
			l.warn("[WARN] Unable to write to mail server: [%s] err: [%v]\n", smtpHost, err)
			return
		}
		l.info("Email sent for app [%s] component [%s]\n", res.App, res.Component)
	}

}
