package telemetry

import (
	"sync"

	"github.com/go-logr/logr"
	"go.opentelemetry.io/otel"
)

// contentFreeErrorHandler drops OpenTelemetry SDK errors. The SDK reports
// export and processing failures through the process-global otel.Handle,
// whose default writes the full error to stderr: exporter URLs, resolved
// collector addresses and collector response bodies. Applications must not
// emit that text, so it is discarded.
type contentFreeErrorHandler struct{}

func (contentFreeErrorHandler) Handle(error) {}

var installDiagnostics sync.Once

// installContentFreeDiagnostics replaces the SDK's process-wide error handler
// and internal logger. The logger receives raw environment values the SDK
// parses (URLs with credentials, header values). Both run before any exporter
// is built, whether or not RegisterGlobal is set and even if Init then fails,
// and are never restored, because restoring the defaults would reintroduce the
// leak. An application that wants SDK diagnostics must install its own
// content-free handler or logger after Init; one installed before is replaced.
func installContentFreeDiagnostics() {
	installDiagnostics.Do(func() {
		otel.SetErrorHandler(contentFreeErrorHandler{})
		otel.SetLogger(logr.Discard())
	})
}
