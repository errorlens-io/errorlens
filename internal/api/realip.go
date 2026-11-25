package api

import (
	"net"
	"net/http"
	"strings"
)

// realIP rewrites r.RemoteAddr from X-Forwarded-For, but only when the
// direct peer is a configured trusted proxy. The header is walked from the
// right, skipping trusted hops, so clients cannot spoof their address.
func realIP(trusted []*net.IPNet) func(http.Handler) http.Handler {
	isTrusted := func(ip net.IP) bool {
		for _, n := range trusted {
			if n.Contains(ip) {
				return true
			}
		}
		return false
	}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if len(trusted) > 0 {
				host, _, err := net.SplitHostPort(r.RemoteAddr)
				if peer := net.ParseIP(host); err == nil && peer != nil && isTrusted(peer) {
					hops := strings.Split(r.Header.Get("X-Forwarded-For"), ",")
					for i := len(hops) - 1; i >= 0; i-- {
						ip := net.ParseIP(strings.TrimSpace(hops[i]))
						if ip == nil {
							break
						}
						if !isTrusted(ip) || i == 0 {
							r.RemoteAddr = net.JoinHostPort(ip.String(), "0")
							break
						}
					}
				}
			}
			next.ServeHTTP(w, r)
		})
	}
}
