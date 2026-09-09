// Package gohttpclient preserves the original HTTP client instrumentation path.
//
// Deprecated: use github.com/faustbrian/go-telemetry/instrumentation/httpclient.
package gohttpclient

import (
	"net/http"

	httpclient "github.com/faustbrian/go-telemetry/instrumentation/httpclient"
)

// Config controls privacy-preserving HTTP client instrumentation.
type Config = httpclient.Config

// Transport instruments outbound HTTP requests.
type Transport = httpclient.Transport

// NewTransport wraps base with privacy-preserving client instrumentation.
func NewTransport(base http.RoundTripper, config Config) (*Transport, error) {
	return httpclient.NewTransport(base, config)
}
