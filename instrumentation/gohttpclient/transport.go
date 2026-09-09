// Package gohttpclient preserves the original HTTP client instrumentation path.
//
// Deprecated: use github.com/faustbrian/go-telemetry/instrumentation/httpclient.
package gohttpclient

import (
	"net/http"

	httpclient "github.com/faustbrian/go-telemetry/instrumentation/httpclient"
)

type Config = httpclient.Config
type Transport = httpclient.Transport

func NewTransport(base http.RoundTripper, config Config) (*Transport, error) {
	return httpclient.NewTransport(base, config)
}
