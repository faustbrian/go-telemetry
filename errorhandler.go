package telemetry

import (
	"sync"

	"go.opentelemetry.io/otel"
)

// contentFreeErrorHandler drops OpenTelemetry SDK errors. The SDK reports
// export and processing failures through the process-global otel.Handle,
// whose default writes the full error to stderr: exporter URLs, resolved
// collector addresses and collector response bodies. Applications must not
// emit that text, so it is discarded.
type contentFreeErrorHandler struct{}

func (contentFreeErrorHandler) Handle(error) {}

var installErrorHandler sync.Once

// installContentFreeErrorHandler replaces the SDK's default error handler for
// the process. It is never restored on Shutdown, because restoring the default
// would reintroduce the leak; an application that wants SDK error signals must
// install its own content-free handler after Init.
func installContentFreeErrorHandler() {
	installErrorHandler.Do(func() { otel.SetErrorHandler(contentFreeErrorHandler{}) })
}
