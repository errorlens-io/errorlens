// Package notify delivers alert notifications (webhook, Slack, Discord, email).
package notify

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"syscall"
	"time"
)

// ErrPrivateAddress is returned when a webhook resolves to a private network.
var ErrPrivateAddress = errors.New("destination is a private or loopback address")

// NewHTTPClient returns a client for user-supplied URLs. Unless allowPrivate is
// set, connections to loopback, private, link-local and metadata addresses are
// refused at dial time (after DNS resolution, so rebinding cannot bypass it).
func NewHTTPClient(allowPrivate bool) *http.Client {
	dialer := &net.Dialer{
		Timeout: 5 * time.Second,
		Control: func(_, address string, _ syscall.RawConn) error {
			if allowPrivate {
				return nil
			}
			host, _, err := net.SplitHostPort(address)
			if err != nil {
				return err
			}
			ip := net.ParseIP(host)
			if ip == nil || isPrivate(ip) {
				return ErrPrivateAddress
			}
			return nil
		},
	}
	return &http.Client{
		Timeout: 10 * time.Second,
		Transport: &http.Transport{
			DialContext:         dialer.DialContext,
			TLSHandshakeTimeout: 5 * time.Second,
			MaxIdleConns:        10,
			Proxy:               nil, // never route user URLs through an env proxy
		},
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= 3 {
				return errors.New("too many redirects")
			}
			return nil
		},
	}
}

func isPrivate(ip net.IP) bool {
	return ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() ||
		ip.IsUnspecified() || ip.IsMulticast() || ip.Equal(net.ParseIP("169.254.169.254")) ||
		// Carrier-grade NAT and IPv4-mapped forms are also internal.
		(ip.To4() != nil && ip.To4()[0] == 100 && ip.To4()[1]&0xC0 == 64)
}

// ValidateURL checks a user-supplied webhook URL's shape.
func ValidateURL(raw string) error {
	u, err := url.Parse(raw)
	if err != nil || (u.Scheme != "https" && u.Scheme != "http") || u.Host == "" {
		return fmt.Errorf("must be an absolute http(s) URL")
	}
	if u.User != nil {
		return fmt.Errorf("must not contain credentials")
	}
	return nil
}

// post sends body to url and treats non-2xx responses as errors.
func post(ctx context.Context, c *http.Client, url, contentType string, body []byte, headers map[string]string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytesReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", contentType)
	req.Header.Set("User-Agent", "ErrorLens-Alerts/1.0")
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	resp, err := c.Do(req)
	if err != nil {
		return err
	}
	resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return fmt.Errorf("destination responded %d", resp.StatusCode)
	}
	return nil
}
