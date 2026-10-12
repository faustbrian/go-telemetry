// Package propagation provides bounded W3C propagation with explicit inbound
// trust decisions.
package propagation

import (
	"context"
	"errors"

	"go.opentelemetry.io/otel/baggage"
	otelpropagation "go.opentelemetry.io/otel/propagation"
)

// Config bounds propagation input and allow-lists baggage keys.
type Config struct {
	// BaggageEnabled opts into trusted extraction and filtered outbound baggage.
	// Extract always clears baggage, including baggage already present in ctx.
	BaggageEnabled bool
	// TrustedBaggageKeys is a finite allow-list, at most 128 unique valid
	// member names of at most 255 bytes. Values/properties are not redacted.
	TrustedBaggageKeys []string
	// MaxHeaderBytes is an inclusive 1..1048576 byte budget: combined
	// traceparent/tracestate input, separate baggage input and serialized baggage output.
	// It is not a limit on allocation performed by TextMapCarrier.Get.
	MaxHeaderBytes int
	// MaxBaggageItems limits retained allowed members to 1..128, not
	// the number of members parsed from an in-budget baggage header.
	MaxBaggageItems int
}

// DefaultConfig returns conservative propagation bounds. Baggage is disabled
// until applications define a trusted key contract.
func DefaultConfig() Config {
	return Config{
		BaggageEnabled:  false,
		MaxHeaderBytes:  8 * 1_024,
		MaxBaggageItems: 16,
	}
}

// Policy implements the standard TextMapPropagator interface. Its Extract
// method treats all inbound carriers as untrusted.
type Policy struct {
	config  Config
	allowed map[string]struct{}
	trace   otelpropagation.TraceContext
	bag     otelpropagation.Baggage
}

// New validates and constructs a propagation policy, building an independent
// allowed-key map. The policy can be reused concurrently with independently safe
// carriers. Carriers and trusted-boundary authentication remain caller-owned.
// Pinned profiles and limits are recorded in TELEMETRY-DEC-003 through 005.
func New(config Config) (*Policy, error) {
	if len(config.TrustedBaggageKeys) > 128 {
		return nil, errors.New("trusted baggage keys exceed 128 entries")
	}
	var errs []error
	if config.MaxHeaderBytes <= 0 || config.MaxHeaderBytes > 1<<20 {
		errs = append(errs, errors.New("maximum propagation header bytes must be between 1 and 1048576"))
	}
	if config.MaxBaggageItems <= 0 || config.MaxBaggageItems > 128 {
		errs = append(errs, errors.New("maximum baggage items must be between 1 and 128"))
	}
	allowed := make(map[string]struct{}, len(config.TrustedBaggageKeys))
	for _, key := range config.TrustedBaggageKeys {
		if len(key) > 255 {
			errs = append(errs, errors.New("trusted baggage key exceeds 255 bytes"))
			continue
		}
		if _, err := baggage.NewMember(key, "value"); err != nil {
			errs = append(errs, errors.New("trusted baggage key is invalid"))
		}
		if _, duplicate := allowed[key]; duplicate {
			errs = append(errs, errors.New("trusted baggage key is duplicated"))
		}
		allowed[key] = struct{}{}
	}
	if err := errors.Join(errs...); err != nil {
		return nil, err
	}
	return &Policy{config: config, allowed: allowed}, nil
}

// Extract applies the untrusted inbound policy: valid W3C trace context is
// accepted within the combined byte budget, while all baggage is removed.
// Invalid/oversized trace input leaves the existing span context unchanged, not
// necessarily empty. Trace identity is correlation, never authentication.
func (p *Policy) Extract(ctx context.Context, carrier otelpropagation.TextMapCarrier) context.Context {
	ctx = p.extractTrace(ctx, carrier)
	return baggage.ContextWithBaggage(ctx, baggage.Baggage{})
}

// ExtractTrusted accepts only allow-listed, bounded baggage from a trusted
// peer while applying the same bounded trace-context parsing. The caller must
// establish trust before invoking it; this method performs no authentication.
// Existing baggage is cleared first. All in-budget input members can be parsed
// before the retained-member cap is applied; allowed values/properties pass through.
func (p *Policy) ExtractTrusted(ctx context.Context, carrier otelpropagation.TextMapCarrier) context.Context {
	ctx = p.extractTrace(ctx, carrier)
	ctx = baggage.ContextWithBaggage(ctx, baggage.Baggage{})
	if !p.config.BaggageEnabled || !withinLimit(carrier, p.config.MaxHeaderBytes, "baggage") {
		return ctx
	}
	extracted := p.bag.Extract(ctx, carrier)
	members := filterMembers(baggage.FromContext(extracted).Members(), p.allowed, p.config.MaxBaggageItems)
	filtered, _ := baggage.New(members...)
	return baggage.ContextWithBaggage(ctx, filtered)
}

// Inject replaces outbound W3C headers and propagates only allow-listed,
// bounded baggage. Clearing uses Set(field, ""), not a guaranteed header deletion.
// Trace serialization is SDK-controlled; the output byte check covers baggage.
// If baggage exceeds its inclusive budget, the entire baggage value is cleared.
// A caller-supplied carrier must not block or mutate concurrently without safety.
func (p *Policy) Inject(ctx context.Context, carrier otelpropagation.TextMapCarrier) {
	carrier.Set("traceparent", "")
	carrier.Set("tracestate", "")
	p.trace.Inject(ctx, carrier)
	carrier.Set("baggage", "")
	if !p.config.BaggageEnabled {
		return
	}
	members := filterMembers(baggage.FromContext(ctx).Members(), p.allowed, p.config.MaxBaggageItems)
	filtered, _ := baggage.New(members...)
	filteredContext := baggage.ContextWithBaggage(ctx, filtered)
	p.bag.Inject(filteredContext, carrier)
	if len(carrier.Get("baggage")) > p.config.MaxHeaderBytes {
		carrier.Set("baggage", "")
	}
}

// Fields returns a fresh SDK registration list. Baggage is listed only when
// enabled, even though Extract removes baggage and Inject clears the carrier
// baggage field when disabled; it is not a complete mutation inventory.
func (p *Policy) Fields() []string {
	fields := []string{"traceparent", "tracestate"}
	if p.config.BaggageEnabled {
		fields = append(fields, "baggage")
	}
	return fields
}

func (p *Policy) extractTrace(ctx context.Context, carrier otelpropagation.TextMapCarrier) context.Context {
	if !withinLimit(carrier, p.config.MaxHeaderBytes, "traceparent", "tracestate") {
		return ctx
	}
	return p.trace.Extract(ctx, carrier)
}

func withinLimit(carrier otelpropagation.TextMapCarrier, limit int, fields ...string) bool {
	total := 0
	for _, field := range fields {
		total += len(carrier.Get(field))
		if total > limit {
			return false
		}
	}
	return true
}

// filterMembers caps retained allowed members after the SDK has parsed the
// already byte-bounded input; it is not a preparse member-admission limit.
func filterMembers(members []baggage.Member, allowed map[string]struct{}, limit int) []baggage.Member {
	filtered := make([]baggage.Member, 0, min(len(members), limit))
	for _, member := range members {
		if _, ok := allowed[member.Key()]; !ok {
			continue
		}
		filtered = append(filtered, member)
		if len(filtered) == limit {
			break
		}
	}
	return filtered
}
