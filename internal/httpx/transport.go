// Package httpx provides a shared, tuned HTTP transport for all outbound
// provider and embedding calls.
package httpx

import (
	"net/http"
	"time"
)

// sharedTransport is reused across every outbound client so connections to the
// same upstream host get pooled. The standard library default caps idle
// connections per host at 2, which throttles keepalive reuse — and forces new
// TLS handshakes — as soon as more than two requests fan in to one provider.
var sharedTransport = &http.Transport{
	Proxy:                 http.ProxyFromEnvironment,
	MaxIdleConns:          200,
	MaxIdleConnsPerHost:   64,
	MaxConnsPerHost:       0, // unlimited in-flight; idle pooling is what matters
	IdleConnTimeout:       90 * time.Second,
	TLSHandshakeTimeout:   10 * time.Second,
	ExpectContinueTimeout: 1 * time.Second,
	ForceAttemptHTTP2:     true,
}

// Client returns an http.Client with the given timeout that shares the tuned
// transport (and therefore the connection pool) with every other caller.
func Client(timeout time.Duration) *http.Client {
	return &http.Client{Timeout: timeout, Transport: sharedTransport}
}
