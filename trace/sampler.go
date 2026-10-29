// Package trace defines stable trace configuration and sampling policies.
package trace

import (
	"errors"
	"fmt"
	"math"

	"go.opentelemetry.io/otel/sdk/trace"
	traceapi "go.opentelemetry.io/otel/trace"
)

// Mode selects the root-span sampling strategy.
type Mode string

const (
	// ModeAlwaysOn samples every root span.
	ModeAlwaysOn Mode = "always_on"
	// ModeAlwaysOff drops every root span.
	ModeAlwaysOff Mode = "always_off"
	// ModeRatio deterministically samples a ratio of trace IDs.
	ModeRatio Mode = "ratio"
)

// Config describes a sampler without replacing the OpenTelemetry sampler API.
type Config struct {
	// Mode selects always-on, always-off or trace-ID ratio root sampling.
	Mode Mode
	// Ratio is checked only in ratio mode and must be finite in [0,1].
	Ratio float64
	// ParentBased delegates sampled/unsampled parent decisions to the SDK;
	// Mode and Ratio then select only the root policy. Sampling is not redaction.
	ParentBased bool
	// ParentDropOnly honors an unsampled parent but applies Mode and Ratio to a
	// sampled parent as to a root, so a caller can lower sampling but never
	// raise it above the local policy. It excludes ParentBased.
	ParentDropOnly bool
}

// NewSampler constructs a standard OpenTelemetry SDK sampler for a supported Mode.
// Ratio is validated only in ratio mode; parent-based wrapping preserves the
// SDK default decisions for local/remote sampled and unsampled parents. Head
// sampling cannot select traces by eventual outcome or guarantee a traffic quota.
// See TestParentBasedSamplerPreservesRemoteDecision; sampling is not privacy.
func NewSampler(config Config) (trace.Sampler, error) {
	var sampler trace.Sampler
	switch config.Mode {
	case ModeAlwaysOn:
		sampler = trace.AlwaysSample()
	case ModeAlwaysOff:
		sampler = trace.NeverSample()
	case ModeRatio:
		if math.IsNaN(config.Ratio) || math.IsInf(config.Ratio, 0) || config.Ratio < 0 || config.Ratio > 1 {
			return nil, fmt.Errorf("sample ratio %f must be between zero and one", config.Ratio)
		}
		sampler = trace.TraceIDRatioBased(config.Ratio)
	default:
		return nil, errors.New("sampling mode is unsupported")
	}
	switch {
	case config.ParentBased && config.ParentDropOnly:
		return nil, errors.New("parent-based and parent-drop-only sampling are exclusive")
	case config.ParentBased:
		sampler = trace.ParentBased(sampler)
	case config.ParentDropOnly:
		sampler = parentDropOnly{local: sampler}
	}
	return sampler, nil
}

// parentDropOnly drops a trace whose parent is unsampled and otherwise defers
// to the local policy, which sees the same trace ID for roots and children.
type parentDropOnly struct{ local trace.Sampler }

func (sampler parentDropOnly) ShouldSample(parameters trace.SamplingParameters) trace.SamplingResult {
	parent := traceapi.SpanContextFromContext(parameters.ParentContext)
	if parent.IsValid() && !parent.IsSampled() {
		return trace.SamplingResult{Decision: trace.Drop, Tracestate: parent.TraceState()}
	}
	return sampler.local.ShouldSample(parameters)
}

func (sampler parentDropOnly) Description() string {
	return "ParentDropOnly{" + sampler.local.Description() + "}"
}
