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

func parentContext(t *testing.T, flags traceapi.TraceFlags, remote bool, traceID traceapi.TraceID) context.Context {
	t.Helper()
	state, err := traceapi.ParseTraceState("vendor=value")
	if err != nil {
		t.Fatal(err)
	}
	parent := traceapi.NewSpanContext(traceapi.SpanContextConfig{
		TraceID: traceID, SpanID: traceapi.SpanID{1}, TraceFlags: flags, TraceState: state, Remote: remote,
	})
	if remote {
		return traceapi.ContextWithRemoteSpanContext(context.Background(), parent)
	}
	return traceapi.ContextWithSpanContext(context.Background(), parent)
}

func TestParentDropOnlyIgnoresTheCallerChosenTraceID(t *testing.T) {
	t.Parallel()

	// A trace ID whose low bytes are zero is always inside TraceIDRatioBased's
	// threshold, so only an independent draw keeps the caller below the ratio.
	lowTraceID := traceapi.TraceID{15: 1}
	for name, test := range map[string]struct {
		draw float64
		want trace.SamplingDecision
	}{
		"draw above the ratio": {draw: 0.5, want: trace.Drop},
		"draw below the ratio": {draw: 0.001, want: trace.RecordAndSample},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			sampler, err := newSampler(Config{Mode: ModeRatio, Ratio: 0.01, ParentDropOnly: true}, func() float64 { return test.draw })
			if err != nil {
				t.Fatal(err)
			}
			result := sampler.ShouldSample(trace.SamplingParameters{
				ParentContext: parentContext(t, traceapi.FlagsSampled, true, lowTraceID), TraceID: lowTraceID, Name: "request",
			})
			if result.Decision != test.want || result.Tracestate.String() != "vendor=value" {
				t.Fatalf("result = (%v, %q), want (%v, vendor=value)", result.Decision, result.Tracestate.String(), test.want)
			}
		})
	}
}

func TestParentDropOnlySamplesSampledCallersAtTheLocalRatio(t *testing.T) {
	t.Parallel()

	const draws = 1000
	next := 0
	sampler, err := newSampler(Config{Mode: ModeRatio, Ratio: 0.25, ParentDropOnly: true}, func() float64 {
		next++
		return float64(next-1) / draws
	})
	if err != nil {
		t.Fatal(err)
	}
	sampled := 0
	for range draws {
		result := sampler.ShouldSample(trace.SamplingParameters{
			ParentContext: parentContext(t, traceapi.FlagsSampled, true, traceapi.TraceID{15: 1}), TraceID: traceapi.TraceID{15: 1},
		})
		if result.Decision == trace.RecordAndSample {
			sampled++
		}
	}
	if sampled != draws/4 {
		t.Fatalf("sampled %d of %d sampled callers, want %d", sampled, draws, draws/4)
	}
}

func TestParentDropOnlyDecisionsByParent(t *testing.T) {
	t.Parallel()

	for name, test := range map[string]struct {
		config Config
		ctx    func(*testing.T) context.Context
		want   trace.SamplingDecision
	}{
		"unsampled remote parent within the ratio": {
			config: Config{Mode: ModeRatio, Ratio: 1},
			ctx:    func(t *testing.T) context.Context { return parentContext(t, 0, true, traceapi.TraceID{1}) },
			want:   trace.Drop,
		},
		"sampled local parent beyond the ratio keeps its decision": {
			config: Config{Mode: ModeRatio, Ratio: 0},
			ctx: func(t *testing.T) context.Context {
				return parentContext(t, traceapi.FlagsSampled, false, traceapi.TraceID{1})
			},
			want: trace.RecordAndSample,
		},
		"unsampled local parent keeps its decision": {
			config: Config{Mode: ModeAlwaysOn},
			ctx:    func(t *testing.T) context.Context { return parentContext(t, 0, false, traceapi.TraceID{1}) },
			want:   trace.Drop,
		},
		"sampled remote parent always on": {
			config: Config{Mode: ModeAlwaysOn},
			ctx: func(t *testing.T) context.Context {
				return parentContext(t, traceapi.FlagsSampled, true, traceapi.TraceID{1})
			},
			want: trace.RecordAndSample,
		},
		"sampled remote parent always off": {
			config: Config{Mode: ModeAlwaysOff},
			ctx: func(t *testing.T) context.Context {
				return parentContext(t, traceapi.FlagsSampled, true, traceapi.TraceID{1})
			},
			want: trace.Drop,
		},
		"root within the ratio": {
			config: Config{Mode: ModeRatio, Ratio: 1},
			ctx:    func(*testing.T) context.Context { return context.Background() },
			want:   trace.RecordAndSample,
		},
		"root beyond the ratio": {
			config: Config{Mode: ModeRatio, Ratio: 0},
			ctx:    func(*testing.T) context.Context { return context.Background() },
			want:   trace.Drop,
		},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			test.config.ParentDropOnly = true
			sampler, err := NewSampler(test.config)
			if err != nil {
				t.Fatal(err)
			}
			result := sampler.ShouldSample(trace.SamplingParameters{ParentContext: test.ctx(t), TraceID: traceapi.TraceID{1}, Name: "child"})
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
