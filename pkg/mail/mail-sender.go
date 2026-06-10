package mail

import (
	"fmt"
	"net/smtp"
	"strings"
)

type EmailSender interface {
	SendHTML(to []string, subject, body string) error
}

type smtpEmailSender struct {
	host     string
	port     string
	email    string
	password string
}

func NewSMTPEmailSender(host, port, email, password string) EmailSender {
	return &smtpEmailSender{
		host:     host,
		port:     port,
		email:    email,
		password: password,
	}
}

func (s *smtpEmailSender) SendHTML(to []string, subject, body string) error {
	if s.host == "" || s.port == "" || s.email == "" {
		return fmt.Errorf("SMTP configuration is incomplete (host=%s, port=%s, email=%s)", s.host, s.port, s.email)
	}

	header := make(map[string]string)
	header["From"] = s.email
	header["To"] = strings.Join(to, ",")
	header["Subject"] = subject
	header["MIME-Version"] = "1.0"
	header["Content-Type"] = "text/html; charset=\"UTF-8\""

	message := ""
	for k, v := range header {
		message += fmt.Sprintf("%s: %s\r\n", k, v)
	}
	message += "\r\n" + body

	auth := smtp.PlainAuth("", s.email, s.password, s.host)
	addr := fmt.Sprintf("%s:%s", s.host, s.port)

	err := smtp.SendMail(addr, auth, s.email, to, []byte(message))
	if err != nil {
		return fmt.Errorf("failed to send email via SMTP: %w", err)
	}

	return nil
}
