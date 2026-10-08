package propagation

import (
	"context"
	"strings"
	"testing"

	otelpropagation "go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/trace"
)

func TestPolicyPreservesExactParentIdentityAndRejectsInvalidIDs(t *testing.T) {
	t.Parallel()
	policy, err := New(DefaultConfig())
	if err != nil {
		t.Fatal(err)
	}
	const traceHex = "4bf92f3577b34da6a3ce929d0e0e4736"
	const spanHex = "00f067aa0ba902b7"
	wantTrace := trace.TraceID{0x4b, 0xf9, 0x2f, 0x35, 0x77, 0xb3, 0x4d, 0xa6, 0xa3, 0xce, 0x92, 0x9d, 0x0e, 0x0e, 0x47, 0x36}
	wantSpan := trace.SpanID{0x00, 0xf0, 0x67, 0xaa, 0x0b, 0xa9, 0x02, 0xb7}
	for _, scenario := range []struct {
		name   string
		header string
		valid  bool
		flags  trace.TraceFlags
	}{
		{name: "sampled", header: "00-" + traceHex + "-" + spanHex + "-01", valid: true, flags: trace.FlagsSampled},
		{name: "unsampled", header: "00-" + traceHex + "-" + spanHex + "-00", valid: true},
		{name: "uppercase trace", header: "00-" + strings.ToUpper(traceHex) + "-" + spanHex + "-01"},
		{name: "uppercase span", header: "00-" + traceHex + "-" + strings.ToUpper(spanHex) + "-01"},
		{name: "zero trace", header: "00-00000000000000000000000000000000-" + spanHex + "-01"},
		{name: "zero span", header: "00-" + traceHex + "-0000000000000000-01"},
		{name: "nonhex trace", header: "00-gbf92f3577b34da6a3ce929d0e0e4736-" + spanHex + "-01"},
		{name: "short trace", header: "00-" + traceHex[1:] + "-" + spanHex + "-01"},
		{name: "invalid flags", header: "00-" + traceHex + "-" + spanHex + "-gg"},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			for _, extractor := range []struct {
				name string
				call func(context.Context, otelpropagation.TextMapCarrier) context.Context
			}{{"untrusted", policy.Extract}, {"trusted", policy.ExtractTrusted}} {
				t.Run(extractor.name, func(t *testing.T) {
					ctx := extractor.call(context.Background(), otelpropagation.MapCarrier{"traceparent": scenario.header})
					got := trace.SpanContextFromContext(ctx)
					outgoing := otelpropagation.MapCarrier{}
					policy.Inject(ctx, outgoing)
					if !scenario.valid {
						if got.IsValid() || got.TraceID() != (trace.TraceID{}) || got.SpanID() != (trace.SpanID{}) || got.TraceFlags() != 0 || got.IsRemote() || outgoing.Get("traceparent") != "" {
							t.Fatal("invalid parent was partially accepted or propagated")
						}
						return
					}
					if !got.IsValid() || got.TraceID() != wantTrace || got.SpanID() != wantSpan || got.TraceFlags() != scenario.flags || !got.IsRemote() {
						t.Fatalf("decoded parent = %v", got)
					}
					if outgoing.Get("traceparent") != scenario.header {
						t.Fatalf("outbound parent = %q, want %q", outgoing.Get("traceparent"), scenario.header)
					}
				})
			}
		})
	}
}
