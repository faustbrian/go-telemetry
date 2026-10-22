package nethttp

import (
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/faustbrian/go-telemetry/v4/testtelemetry"
)

func TestTransportComposesWithStandardRoundTripper(t *testing.T) {
	t.Parallel()

	harness := testtelemetry.New()
	transport, err := NewTransport(roundTripperFunc(func(request *http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusNoContent,
			Header:     make(http.Header),
			Body:       io.NopCloser(strings.NewReader("")),
			Request:    request,
		}, nil
	}), ClientConfig{
		Operation:      "vendor.request",
		TracerProvider: harness.TracerProvider(),
		MeterProvider:  harness.MeterProvider(),
	})
	if err != nil {
		t.Fatalf("NewTransport() error = %v", err)
	}
	request, _ := http.NewRequest(http.MethodGet, "https://vendor.example/secret", nil)
	response, err := transport.RoundTrip(request)
	if err != nil {
		t.Fatalf("RoundTrip() error = %v", err)
	}
	_ = response.Body.Close()
	if spans := harness.Spans(); len(spans) != 1 || spans[0].Name != "vendor.request" {
		t.Fatalf("spans = %+v, want vendor.request", spans)
	}
}
