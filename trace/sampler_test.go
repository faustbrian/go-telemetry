package trace

import (
	"context"
	"math"
	"strings"
	"testing"

	"go.opentelemetry.io/otel/sdk/trace"
	traceapi "go.opentelemetry.io/otel/trace"
)

func TestParentBasedSamplerPreservesRemoteDecision(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		parentFlag traceapi.TraceFlags
		ratio      float64
		want       trace.SamplingDecision
	}{
		{name: "sampled parent", parentFlag: traceapi.FlagsSampled, ratio: 0, want: trace.RecordAndSample},
		{name: "unsampled parent", parentFlag: 0, ratio: 1, want: trace.Drop},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			sampler, err := NewSampler(Config{Mode: ModeRatio, Ratio: test.ratio, ParentBased: true})
			if err != nil {
				t.Fatalf("NewSampler() error = %v", err)
			}
			parent := traceapi.NewSpanContext(traceapi.SpanContextConfig{
				TraceID:    traceapi.TraceID{1},
				SpanID:     traceapi.SpanID{1},
				TraceFlags: test.parentFlag,
				Remote:     true,
			})
			result := sampler.ShouldSample(trace.SamplingParameters{
				ParentContext: traceapi.ContextWithRemoteSpanContext(context.Background(), parent),
				TraceID:       parent.TraceID(),
				Name:          "child",
			})
			if result.Decision != test.want {
				t.Fatalf("sampling decision = %v, want %v", result.Decision, test.want)
			}
		})
	}
}

func TestNewSamplerValidatesModesAndRatio(t *testing.T) {
	t.Parallel()

	for _, config := range []Config{
		{Mode: Mode("unknown")},
		{Mode: ModeRatio, Ratio: -0.1},
		{Mode: ModeRatio, Ratio: 1.1},
		{Mode: ModeRatio, Ratio: math.NaN()},
		{Mode: ModeRatio, Ratio: math.Inf(1)},
	} {
		if _, err := NewSampler(config); err == nil {
			t.Fatalf("NewSampler(%+v) error = nil, want validation error", config)
		}
	}
}

func TestNewSamplerRejectsModeWithoutDisclosingItsValue(t *testing.T) {
	const sensitive = "password=private_payload"
	if _, err := NewSampler(Config{Mode: Mode(sensitive)}); err == nil || strings.Contains(err.Error(), sensitive) {
		t.Fatal("unsupported sampling mode was accepted or disclosed")
	}
}

func TestSamplerModes(t *testing.T) {
	t.Parallel()

	for _, test := range []struct {
		mode Mode
		want trace.SamplingDecision
	}{
		{mode: ModeAlwaysOn, want: trace.RecordAndSample},
		{mode: ModeAlwaysOff, want: trace.Drop},
	} {
		sampler, err := NewSampler(Config{Mode: test.mode})
		if err != nil {
			t.Fatalf("NewSampler() error = %v", err)
		}
		result := sampler.ShouldSample(trace.SamplingParameters{TraceID: traceapi.TraceID{1}})
		if result.Decision != test.want {
			t.Fatalf("mode %q decision = %v, want %v", test.mode, result.Decision, test.want)
		}
	}
}

func TestParentDropOnlySamplerLetsCallersLowerButNotRaiseSampling(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		parent *traceapi.TraceFlags
		ratio  float64
		want   trace.SamplingDecision
	}{
		{name: "sampled parent beyond the local ratio", parent: new(traceapi.FlagsSampled), ratio: 0, want: trace.Drop},
		{name: "sampled parent within the local ratio", parent: new(traceapi.FlagsSampled), ratio: 1, want: trace.RecordAndSample},
		{name: "unsampled parent", parent: new(traceapi.TraceFlags(0)), ratio: 1, want: trace.Drop},
		{name: "root within the local ratio", ratio: 1, want: trace.RecordAndSample},
		{name: "root beyond the local ratio", ratio: 0, want: trace.Drop},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			sampler, err := NewSampler(Config{Mode: ModeRatio, Ratio: test.ratio, ParentDropOnly: true})
			if err != nil {
				t.Fatalf("NewSampler() error = %v", err)
			}
			ctx := context.Background()
			if test.parent != nil {
				parent := traceapi.NewSpanContext(traceapi.SpanContextConfig{
					TraceID: traceapi.TraceID{1}, SpanID: traceapi.SpanID{1}, TraceFlags: *test.parent, Remote: true,
				})
				ctx = traceapi.ContextWithRemoteSpanContext(ctx, parent)
			}
			result := sampler.ShouldSample(trace.SamplingParameters{ParentContext: ctx, TraceID: traceapi.TraceID{1}, Name: "child"})
			if result.Decision != test.want {
				t.Fatalf("sampling decision = %v, want %v", result.Decision, test.want)
			}
		})
	}
}

func TestParentDropOnlyExcludesParentBased(t *testing.T) {
	t.Parallel()

	if _, err := NewSampler(Config{Mode: ModeAlwaysOn, ParentBased: true, ParentDropOnly: true}); err == nil {
		t.Fatal("NewSampler() accepted both parent policies")
	}
}
