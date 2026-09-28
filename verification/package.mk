.PHONY: compatibility examples integration

examples:
	go build ./examples/...

integration:
	go test -run 'CollectorInteroperability|ExporterFailureModes' ./otlp

compatibility:
	./scripts/test-otel-version.sh v1.45.0
