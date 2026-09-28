package telemetry_test

import (
	"reflect"
	"testing"

	service "github.com/faustbrian/go-telemetry/v2/adapters/service"
	cache "github.com/faustbrian/go-telemetry/v2/instrumentation/cache"
	legacycache "github.com/faustbrian/go-telemetry/v2/instrumentation/gocache"

	//lint:ignore SA1019 Compatibility identity requires the legacy path.
	legacyhttpclient "github.com/faustbrian/go-telemetry/v2/instrumentation/gohttpclient" //nolint:staticcheck // Compatibility identity requires the legacy path.

	legacypostgres "github.com/faustbrian/go-telemetry/v2/instrumentation/gopostgres"
	legacyqueue "github.com/faustbrian/go-telemetry/v2/instrumentation/goqueue"
	legacyruntime "github.com/faustbrian/go-telemetry/v2/instrumentation/goruntime"
	httpclient "github.com/faustbrian/go-telemetry/v2/instrumentation/httpclient"
	postgres "github.com/faustbrian/go-telemetry/v2/instrumentation/postgres"
	queue "github.com/faustbrian/go-telemetry/v2/instrumentation/queue"
	runtime "github.com/faustbrian/go-telemetry/v2/instrumentation/runtime"
	legacyservice "github.com/faustbrian/go-telemetry/v2/telemetryservice"
)

func TestSuccessorsPreserveLegacyTypeAndSentinelIdentity(t *testing.T) {
	t.Parallel()

	types := []struct {
		successor any
		legacy    any
		pkgPath   string
	}{
		{cache.Config{}, legacycache.Config{}, "github.com/faustbrian/go-telemetry/v2/instrumentation/gocache"},
		{cache.Instrumenter{}, legacycache.Instrumenter{}, "github.com/faustbrian/go-telemetry/v2/instrumentation/gocache"},
		{httpclient.Config{}, legacyhttpclient.Config{}, "github.com/faustbrian/go-telemetry/v2/instrumentation/nethttp"},
		{httpclient.Transport{}, legacyhttpclient.Transport{}, "github.com/faustbrian/go-telemetry/v2/instrumentation/nethttp"},
		{postgres.Config{}, legacypostgres.Config{}, "github.com/faustbrian/go-telemetry/v2/instrumentation/gopostgres"},
		{postgres.Tracer{}, legacypostgres.Tracer{}, "github.com/faustbrian/go-telemetry/v2/instrumentation/gopostgres"},
		{queue.Config{}, legacyqueue.Config{}, "github.com/faustbrian/go-telemetry/v2/instrumentation/goqueue"},
		{queue.Instrumenter{}, legacyqueue.Instrumenter{}, "github.com/faustbrian/go-telemetry/v2/instrumentation/goqueue"},
		{runtime.Instrumenter{}, legacyruntime.Instrumenter{}, "github.com/faustbrian/go-telemetry/v2/instrumentation/goruntime"},
		{service.Options{}, legacyservice.Options{}, "github.com/faustbrian/go-telemetry/v2/telemetryservice"},
		{service.Adapter{}, legacyservice.Adapter{}, "github.com/faustbrian/go-telemetry/v2/telemetryservice"},
	}
	for _, pair := range types {
		if reflect.TypeOf(pair.successor) != reflect.TypeOf(pair.legacy) {
			t.Fatalf("successor type %T differs from legacy type %T", pair.successor, pair.legacy)
		}
		if got := reflect.TypeOf(pair.legacy).PkgPath(); got != pair.pkgPath {
			t.Fatalf("legacy type %T package path = %q, want %q", pair.legacy, got, pair.pkgPath)
		}
	}
	//nolint:errorlint // Exact sentinel identity is the compatibility contract.
	if service.ErrInvalidOptions != legacyservice.ErrInvalidOptions {
		t.Fatal("service sentinel identity changed")
	}
}
