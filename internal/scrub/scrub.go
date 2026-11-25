// Package scrub removes sensitive data from events before they are stored.
//
// It runs in the ingest path, so raw secrets never reach the database, the
// job queue or logs. It works on decoded JSON (map[string]any / []any).
package scrub

import (
	"net"
	"net/url"
	"regexp"
	"strings"
)

// Filtered replaces every scrubbed value.
const Filtered = "[Filtered]"

// IPMode controls what happens to user IP addresses.
type IPMode string

const (
	IPKeep     IPMode = "keep"
	IPTruncate IPMode = "truncate" // IPv4 /24, IPv6 /48
	IPDrop     IPMode = "drop"
)

// Config is a project's scrubbing configuration.
type Config struct {
	Defaults        bool     // apply the built-in key and value rules
	IP              IPMode   // user IP handling
	SensitiveFields []string // extra key substrings to filter
	SafeFields      []string // exact keys never filtered by key rules
}

// DefaultConfig is used for new projects.
var DefaultConfig = Config{Defaults: true, IP: IPTruncate}

// defaultKeys are matched as case-insensitive substrings of normalized keys
// (lower-case, '-' and '_' removed).
var defaultKeys = []string{
	"password", "passwd", "pwd", "secret", "token", "apikey", "accesskey",
	"auth", "credential", "cookie", "session", "csrf", "xsrf", "privatekey",
	"creditcard", "cardnumber", "cvv", "cvc", "ssn", "iban", "signature",
}

// valuePatterns detect secrets inside arbitrary strings.
var valuePatterns = []*regexp.Regexp{
	// PEM private keys
	regexp.MustCompile(`-----BEGIN [A-Z ]*PRIVATE KEY-----[\s\S]*?-----END [A-Z ]*PRIVATE KEY-----`),
	// JSON Web Tokens
	regexp.MustCompile(`eyJ[A-Za-z0-9_-]{5,}\.eyJ[A-Za-z0-9_-]{5,}\.[A-Za-z0-9_-]*`),
	// Authorization schemes
	regexp.MustCompile(`(?i)\b(bearer|basic|token)\s+[A-Za-z0-9._~+/=-]{8,}`),
	// AWS access key ids
	regexp.MustCompile(`\b(AKIA|ASIA)[0-9A-Z]{16}\b`),
	// GitHub tokens
	regexp.MustCompile(`\b(gh[pousr]_[A-Za-z0-9]{36,}|github_pat_[A-Za-z0-9_]{40,})\b`),
	// Stripe live/test secret and restricted keys
	regexp.MustCompile(`\b(sk|rk)_(live|test)_[A-Za-z0-9]{16,}\b`),
	// Slack tokens
	regexp.MustCompile(`\bxox[abposr]-[A-Za-z0-9-]{10,}\b`),
	// Google API keys
	regexp.MustCompile(`\bAIza[0-9A-Za-z_-]{35}\b`),
}

// Password in URLs, e.g. postgres://user:secret@host/db
var urlPassword = regexp.MustCompile(`([a-zA-Z][a-zA-Z0-9+.-]*://[^\s:/@]+:)([^\s@/]+)(@)`)

// key=value pairs in free text, e.g. "login failed password=hunter2"
var kvSecret = regexp.MustCompile(`(?i)\b(password|passwd|pwd|secret|token|api[_-]?key|access[_-]?key)(\s*[=:]\s*)("[^"]*"|'[^']*'|[^\s&,;]+)`)

// Candidate card numbers: 13–19 digits with optional spaces/dashes.
var cardCandidate = regexp.MustCompile(`\b(?:\d[ -]?){12,18}\d\b`)

// Scrubber applies a Config.
type Scrubber struct {
	cfg       Config
	sensitive []string
	safe      map[string]bool
	Count     int // number of values filtered by the last Event call
}

// New returns a scrubber for cfg.
func New(cfg Config) *Scrubber {
	s := &Scrubber{cfg: cfg, safe: map[string]bool{}}
	if cfg.Defaults {
		s.sensitive = append(s.sensitive, defaultKeys...)
	}
	for _, k := range cfg.SensitiveFields {
		if k = normKey(k); k != "" {
			s.sensitive = append(s.sensitive, k)
		}
	}
	for _, k := range cfg.SafeFields {
		s.safe[normKey(k)] = true
	}
	return s
}

func normKey(k string) string {
	k = strings.ToLower(strings.TrimSpace(k))
	return strings.NewReplacer("-", "", "_", "", " ", "", ".", "").Replace(k)
}

func (s *Scrubber) sensitiveKey(k string) bool {
	n := normKey(k)
	if n == "" || s.safe[n] {
		return false
	}
	for _, p := range s.sensitive {
		if strings.Contains(n, p) {
			return true
		}
	}
	return false
}

// Event scrubs a decoded SDK event in place and returns it.
// Count reports how many values were filtered.
func (s *Scrubber) Event(ev map[string]any) map[string]any {
	s.Count = 0
	s.scrubIP(ev)
	for k, v := range ev {
		switch k {
		// Structural fields that never contain user data; skipping them also
		// keeps grouping stable.
		case "event_id", "timestamp", "start_timestamp", "level", "platform", "sdk", "fingerprint", "type", "release", "environment":
			continue
		}
		ev[k] = s.walk(k, v)
	}
	return ev
}

// walk scrubs v, which was found under key.
func (s *Scrubber) walk(key string, v any) any {
	if key != "" && s.sensitiveKey(key) {
		if isEmpty(v) {
			return v
		}
		s.Count++
		return Filtered
	}
	switch t := v.(type) {
	case map[string]any:
		for k, child := range t {
			t[k] = s.walk(k, child)
		}
		return t
	case []any:
		// SDKs encode headers/cookies/query as [[key, value], ...] lists.
		if len(t) == 2 {
			if k, ok := t[0].(string); ok && len(k) < 64 && !strings.ContainsAny(k, " \t\n=(") {
				if _, isPairList := t[1].(string); isPairList && s.sensitiveKey(k) {
					s.Count++
					return []any{k, Filtered}
				}
			}
		}
		for i, child := range t {
			t[i] = s.walk("", child)
		}
		return t
	case string:
		if key == "query_string" || strings.Contains(t, "=") && strings.Contains(t, "&") {
			t = s.query(t)
		}
		return s.String(t)
	default:
		return v
	}
}

func isEmpty(v any) bool {
	switch t := v.(type) {
	case nil:
		return true
	case string:
		return t == ""
	}
	return false
}

// query scrubs sensitive parameters in a URL query string.
func (s *Scrubber) query(q string) string {
	vals, err := url.ParseQuery(strings.TrimPrefix(q, "?"))
	if err != nil || len(vals) == 0 {
		return q
	}
	changed := false
	for k := range vals {
		if s.sensitiveKey(k) {
			vals[k] = []string{Filtered}
			s.Count++
			changed = true
		}
	}
	if !changed {
		return q
	}
	return vals.Encode()
}

// String applies value rules to a single string.
func (s *Scrubber) String(v string) string {
	if !s.cfg.Defaults || len(v) < 8 {
		return v
	}
	for _, re := range valuePatterns {
		if re.MatchString(v) {
			v = re.ReplaceAllStringFunc(v, func(string) string { s.Count++; return Filtered })
		}
	}
	if urlPassword.MatchString(v) {
		v = urlPassword.ReplaceAllStringFunc(v, func(m string) string {
			s.Count++
			parts := urlPassword.FindStringSubmatch(m)
			return parts[1] + Filtered + parts[3]
		})
	}
	if kvSecret.MatchString(v) {
		v = kvSecret.ReplaceAllStringFunc(v, func(m string) string {
			parts := kvSecret.FindStringSubmatch(m)
			if parts[3] == Filtered {
				return m
			}
			s.Count++
			return parts[1] + parts[2] + Filtered
		})
	}
	if cardCandidate.MatchString(v) {
		v = cardCandidate.ReplaceAllStringFunc(v, func(m string) string {
			if luhn(m) {
				s.Count++
				return Filtered
			}
			return m
		})
	}
	return v
}

func luhn(s string) bool {
	sum, n, double := 0, 0, false
	for i := len(s) - 1; i >= 0; i-- {
		c := s[i]
		if c == ' ' || c == '-' {
			continue
		}
		d := int(c - '0')
		if double {
			if d *= 2; d > 9 {
				d -= 9
			}
		}
		sum += d
		n++
		double = !double
	}
	return n >= 13 && sum%10 == 0
}

// scrubIP applies the IP policy to user.ip_address and request.env.REMOTE_ADDR.
func (s *Scrubber) scrubIP(ev map[string]any) {
	apply := func(m map[string]any, key string) {
		raw, ok := m[key].(string)
		if !ok || raw == "" {
			return
		}
		switch s.cfg.IP {
		case IPKeep:
			if raw == "{{auto}}" {
				delete(m, key) // we never infer the client's IP
			}
		case IPDrop:
			delete(m, key)
			s.Count++
		default: // truncate
			if t := TruncateIP(raw); t != "" {
				m[key] = t
			} else {
				delete(m, key)
			}
		}
	}
	if u, ok := ev["user"].(map[string]any); ok {
		apply(u, "ip_address")
	}
	if r, ok := ev["request"].(map[string]any); ok {
		if env, ok := r["env"].(map[string]any); ok {
			apply(env, "REMOTE_ADDR")
		}
	}
}

// TruncateIP zeroes the host part of an IP (IPv4 /24, IPv6 /48).
// It returns "" for values that are not IP addresses.
func TruncateIP(raw string) string {
	ip := net.ParseIP(strings.TrimSpace(raw))
	if ip == nil {
		return ""
	}
	if v4 := ip.To4(); v4 != nil {
		return net.IPv4(v4[0], v4[1], v4[2], 0).String()
	}
	return ip.Mask(net.CIDRMask(48, 128)).String()
}
