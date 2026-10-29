// Package trace defines stable trace configuration and sampling policies.
package trace

import (
	"errors"
	"fmt"
	"math"
	"math/rand/v2"

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
	// ParentDropOnly treats remote callers as untrusted: an unsampled remote
	// parent drops, and a sampled remote parent is sampled again by an
	// independent random draw at the local policy, so a caller can lower but
	// never raise sampling, whatever trace ID it sends. Local parents keep their
	// decision and roots use Mode and Ratio. It excludes ParentBased.
	ParentDropOnly bool
}

// NewSampler constructs a standard OpenTelemetry SDK sampler for a supported Mode.
// Ratio is validated only in ratio mode; parent-based wrapping preserves the
// SDK default decisions for local/remote sampled and unsampled parents, and
// ParentDropOnly caps sampled remote parents at the local policy. Head
// sampling cannot select traces by eventual outcome or guarantee a traffic quota.
// See TestParentBasedSamplerPreservesRemoteDecision; sampling is not privacy.
func NewSampler(config Config) (trace.Sampler, error) {
	return newSampler(config, rand.Float64)
}

func newSampler(config Config, draw func() float64) (trace.Sampler, error) {
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
		sampler = trace.ParentBased(sampler, trace.WithRemoteParentSampled(remoteCap{config: config, draw: draw}))
	}
	return sampler, nil
}

// remoteCap decides a sampled remote parent by a fresh draw rather than its
// trace ID: ratio sampling reads the trace ID, which a remote caller chooses,
// so the caller could otherwise force sampling with a low ID.
type remoteCap struct {
	config Config
	draw   func() float64
}

func (sampler remoteCap) ShouldSample(parameters trace.SamplingParameters) trace.SamplingResult {
	sampled := sampler.config.Mode == ModeAlwaysOn ||
		sampler.config.Mode == ModeRatio && sampler.draw() < sampler.config.Ratio
	decision := trace.Drop
	if sampled {
		decision = trace.RecordAndSample
	}
	return trace.SamplingResult{
		Decision:   decision,
		Tracestate: traceapi.SpanContextFromContext(parameters.ParentContext).TraceState(),
	}
}

func (sampler remoteCap) Description() string {
	return fmt.Sprintf("RemoteParentCap{%s,%g}", sampler.config.Mode, sampler.config.Ratio)
}
