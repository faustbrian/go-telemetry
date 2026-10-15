#!/bin/sh
set -eu

version=${1:?usage: test-otel-version.sh VERSION}

task=$(mktemp -d "${TMPDIR:-/tmp}/telemetry-otel-profile.XXXXXX")
trap 'rm -rf "$task"' EXIT
modfile=$task/profile.mod
cp go.mod "$modfile"
if [ -f go.sum ]; then cp go.sum "$task/profile.sum"; fi
export GOWORK=off
export GOCACHE=${GOCACHE:-$task/cache}
export GOMODCACHE=${GOMODCACHE:-$task/modules}
export GOTMPDIR=${GOTMPDIR:-$task/tmp}
mkdir -p "$GOCACHE" "$GOMODCACHE" "$GOTMPDIR"

modules='go.opentelemetry.io/otel
go.opentelemetry.io/otel/metric
go.opentelemetry.io/otel/trace
go.opentelemetry.io/otel/sdk
go.opentelemetry.io/otel/sdk/metric
go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc
go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp
go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetricgrpc
go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetrichttp'
for module in $modules
do
    go mod edit "-modfile=$modfile" "-require=${module}@${version}"
done

# This unused selected requirement otherwise uplifts the minimum API graph.
go mod edit "-modfile=$modfile" -droprequire=go.opentelemetry.io/otel/log
go mod tidy "-modfile=$modfile"
resolved=$(go list "-modfile=$modfile" -m -f '{{.Path}} {{.Version}} {{if .Replace}}replaced{{else}}direct{{end}}' $modules)
for module in $modules
do
    if ! printf '%s\n' "$resolved" | grep -Fxq "$module $version direct"; then
        printf 'requested %s@%s, resolved graph:\n%s\n' "$module" "$version" "$resolved" >&2
        exit 1
    fi
done
printf 'Go toolchain: %s\n%s\n' "$(go env GOVERSION)" "$resolved"
profile=selected
case "$version" in v1.46.*) profile=minimum-common ;; esac
TELEMETRY_OTEL_TEST_PROFILE=$profile go test "-modfile=$modfile" ./...
