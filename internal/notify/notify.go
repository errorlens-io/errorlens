package notify

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/smtp"
	"strings"
	"time"

	"github.com/rootcausehq/rootcause/internal/config"
)

// Message is a rendered alert.
type Message struct {
	Subject  string    `json:"subject"`
	Reason   string    `json:"reason"` // "New issue", "Regression", "Spike: 120 events in 5m"
	Title    string    `json:"title"`
	Culprit  string    `json:"culprit"`
	Project  string    `json:"project"`
	Level    string    `json:"level"`
	URL      string    `json:"url"`
	IssueID  int64     `json:"issueId"`
	Count    int64     `json:"count"`
	FiredAt  time.Time `json:"firedAt"`
	RuleName string    `json:"rule"`
	IsTest   bool      `json:"test,omitempty"`
	Org      string    `json:"organization"`
}

// Channel is an alert destination.
type Channel struct {
	Kind   string
	Config ChannelConfig
}

// ChannelConfig holds destination settings.
type ChannelConfig struct {
	URL    string   `json:"url,omitempty"`
	Secret string   `json:"secret,omitempty"` // webhook HMAC secret
	Emails []string `json:"emails,omitempty"`
}

// Sender delivers messages.
type Sender struct {
	HTTP *http.Client
	SMTP config.SMTP
}

// ErrEmailDisabled is returned when SMTP is not configured.
var ErrEmailDisabled = errors.New("email is not configured on this server (set ROOTCAUSE_SMTP_HOST)")

func bytesReader(b []byte) io.Reader { return bytes.NewReader(b) }

// Send delivers m to ch.
func (s *Sender) Send(ctx context.Context, ch Channel, m Message) error {
	switch ch.Kind {
	case "webhook":
		body, _ := json.Marshal(map[string]any{"event": "issue.alert", "data": m})
		headers := map[string]string{}
		if ch.Config.Secret != "" {
			mac := hmac.New(sha256.New, []byte(ch.Config.Secret))
			mac.Write(body)
			headers["X-RootCause-Signature"] = "sha256=" + hex.EncodeToString(mac.Sum(nil))
		}
		return post(ctx, s.HTTP, ch.Config.URL, "application/json", body, headers)
	case "slack":
		text := fmt.Sprintf("*%s* · %s\n<%s|%s>\n`%s` · %s · %s", m.Reason, m.Project, m.URL, escSlack(m.Title), escSlack(m.Culprit), m.Level, plural(m.Count, "event"))
		body, _ := json.Marshal(map[string]any{"text": text, "unfurl_links": false})
		return post(ctx, s.HTTP, ch.Config.URL, "application/json", body, nil)
	case "discord":
		content := fmt.Sprintf("**%s** · %s\n[%s](%s)\n`%s` · %s · %s", m.Reason, m.Project, clip(m.Title, 300), m.URL, clip(m.Culprit, 200), m.Level, plural(m.Count, "event"))
		body, _ := json.Marshal(map[string]any{"content": clip(content, 1900), "allowed_mentions": map[string]any{"parse": []string{}}})
		return post(ctx, s.HTTP, ch.Config.URL, "application/json", body, nil)
	case "email":
		return s.email(ch.Config.Emails, m)
	}
	return fmt.Errorf("unknown channel kind %q", ch.Kind)
}

func (s *Sender) email(to []string, m Message) error {
	if s.SMTP.Host == "" {
		return ErrEmailDisabled
	}
	if len(to) == 0 {
		return errors.New("no recipients")
	}
	body := fmt.Sprintf("%s in %s\n\n%s\n%s\n\nLevel: %s\nEvents: %d\nRule: %s\n\nOpen: %s\n",
		m.Reason, m.Project, m.Title, m.Culprit, m.Level, m.Count, m.RuleName, m.URL)
	return s.SendMail(to, m.Subject, body)
}

// SendMail sends a plain-text email.
func (s *Sender) SendMail(to []string, subject, body string) error {
	if s.SMTP.Host == "" {
		return ErrEmailDisabled
	}
	for _, addr := range to {
		if strings.ContainsAny(addr, "\r\n") {
			return errors.New("invalid recipient")
		}
	}
	subject = strings.NewReplacer("\r", " ", "\n", " ").Replace(subject)
	msg := "From: " + s.SMTP.From + "\r\n" +
		"To: " + strings.Join(to, ", ") + "\r\n" +
		"Subject: " + subject + "\r\n" +
		"MIME-Version: 1.0\r\n" +
		"Content-Type: text/plain; charset=utf-8\r\n" +
		"Date: " + time.Now().Format(time.RFC1123Z) + "\r\n\r\n" +
		strings.ReplaceAll(body, "\n", "\r\n")
	var auth smtp.Auth
	if s.SMTP.Username != "" {
		auth = smtp.PlainAuth("", s.SMTP.Username, s.SMTP.Password, s.SMTP.Host)
	}
	from := s.SMTP.From
	if i := strings.LastIndex(from, "<"); i >= 0 {
		from = strings.TrimSuffix(from[i+1:], ">")
	}
	return smtp.SendMail(fmt.Sprintf("%s:%d", s.SMTP.Host, s.SMTP.Port), auth, from, to, []byte(msg))
}

func escSlack(s string) string {
	return strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;").Replace(clip(s, 300))
}

func clip(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}

func plural(n int64, word string) string {
	if n == 1 {
		return "1 " + word
	}
	return fmt.Sprintf("%d %ss", n, word)
}
