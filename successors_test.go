package telemetry_test

import (
	"reflect"
	"testing"

	service "github.com/faustbrian/go-telemetry/adapters/service"
	cache "github.com/faustbrian/go-telemetry/instrumentation/cache"
	legacycache "github.com/faustbrian/go-telemetry/instrumentation/gocache"
	legacyhttpclient "github.com/faustbrian/go-telemetry/instrumentation/gohttpclient"
	legacypostgres "github.com/faustbrian/go-telemetry/instrumentation/gopostgres"
	legacyqueue "github.com/faustbrian/go-telemetry/instrumentation/goqueue"
	legacyruntime "github.com/faustbrian/go-telemetry/instrumentation/goruntime"
	httpclient "github.com/faustbrian/go-telemetry/instrumentation/httpclient"
	postgres "github.com/faustbrian/go-telemetry/instrumentation/postgres"
	queue "github.com/faustbrian/go-telemetry/instrumentation/queue"
	runtime "github.com/faustbrian/go-telemetry/instrumentation/runtime"
	legacyservice "github.com/faustbrian/go-telemetry/telemetryservice"
)

func TestSuccessorsPreserveLegacyTypeAndSentinelIdentity(t *testing.T) {
	t.Parallel()

	types := []struct {
		successor any
		legacy    any
		pkgPath   string
	}{
		{cache.Config{}, legacycache.Config{}, "github.com/faustbrian/go-telemetry/instrumentation/gocache"},
		{cache.Instrumenter{}, legacycache.Instrumenter{}, "github.com/faustbrian/go-telemetry/instrumentation/gocache"},
		{httpclient.Config{}, legacyhttpclient.Config{}, "github.com/faustbrian/go-telemetry/instrumentation/nethttp"},
		{httpclient.Transport{}, legacyhttpclient.Transport{}, "github.com/faustbrian/go-telemetry/instrumentation/nethttp"},
		{postgres.Config{}, legacypostgres.Config{}, "github.com/faustbrian/go-telemetry/instrumentation/gopostgres"},
		{postgres.Tracer{}, legacypostgres.Tracer{}, "github.com/faustbrian/go-telemetry/instrumentation/gopostgres"},
		{queue.Config{}, legacyqueue.Config{}, "github.com/faustbrian/go-telemetry/instrumentation/goqueue"},
		{queue.Instrumenter{}, legacyqueue.Instrumenter{}, "github.com/faustbrian/go-telemetry/instrumentation/goqueue"},
		{runtime.Instrumenter{}, legacyruntime.Instrumenter{}, "github.com/faustbrian/go-telemetry/instrumentation/goruntime"},
		{service.Options{}, legacyservice.Options{}, "github.com/faustbrian/go-telemetry/telemetryservice"},
		{service.Adapter{}, legacyservice.Adapter{}, "github.com/faustbrian/go-telemetry/telemetryservice"},
	}
	for _, pair := range types {
		if reflect.TypeOf(pair.successor) != reflect.TypeOf(pair.legacy) {
			t.Fatalf("successor type %T differs from legacy type %T", pair.successor, pair.legacy)
		}
		if got := reflect.TypeOf(pair.legacy).PkgPath(); got != pair.pkgPath {
			t.Fatalf("legacy type %T package path = %q, want %q", pair.legacy, got, pair.pkgPath)
		}
	}
	if service.ErrInvalidOptions != legacyservice.ErrInvalidOptions {
		t.Fatal("service sentinel identity changed")
	}
}
