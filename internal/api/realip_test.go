package api

import (
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestRealIP(t *testing.T) {
	_, proxy, _ := net.ParseCIDR("10.0.0.0/8")
	cases := []struct {
		trusted []*net.IPNet
		remote  string
		xff     string
		want    string
	}{
		{nil, "1.2.3.4:5", "9.9.9.9", "1.2.3.4"},                                     // no proxies configured: ignore header
		{[]*net.IPNet{proxy}, "1.2.3.4:5", "9.9.9.9", "1.2.3.4"},                     // untrusted peer: ignore header
		{[]*net.IPNet{proxy}, "10.0.0.2:5", "9.9.9.9", "9.9.9.9"},                    // trusted proxy
		{[]*net.IPNet{proxy}, "10.0.0.2:5", "6.6.6.6, 9.9.9.9, 10.0.0.7", "9.9.9.9"}, // spoofed left part ignored
	}
	for _, c := range cases {
		var got string
		h := realIP(c.trusted)(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) { got = clientIP(r) }))
		req := httptest.NewRequest("GET", "/", nil)
		req.RemoteAddr = c.remote
		req.Header.Set("X-Forwarded-For", c.xff)
		h.ServeHTTP(httptest.NewRecorder(), req)
		if got != c.want {
			t.Errorf("remote=%s xff=%q: got %s want %s", c.remote, c.xff, got, c.want)
		}
	}
}
