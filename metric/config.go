// Package metric defines bounded metric SDK configuration, views, and
// cardinality controls.
package metric

import (
	"errors"
	"fmt"
	"math"
	"regexp"
	"unicode/utf8"

	"go.opentelemetry.io/otel/attribute"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
)

var instrumentNamePattern = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9_.\-/*?]{0,254}$`)

const (
	maxViews             = 128
	maxAllowedAttributes = 128
	maxBoundaries        = 256
)

// Config applies a hard per-stream cardinality budget and explicit views.
type Config struct {
	CardinalityLimit int
	Views            []ViewConfig
}

// ViewConfig controls one metric stream. AllowedAttributes is an allow-list;
// an empty list records no attributes for the matching stream.
type ViewConfig struct {
	Name              string
	Unit              string
	AllowedAttributes []attribute.Key
	Boundaries        []float64
	NoMinMax          bool
}

// Options validates config and returns standard OpenTelemetry SDK options.
func Options(config Config) ([]sdkmetric.Option, error) {
	if config.CardinalityLimit <= 0 || config.CardinalityLimit > 100_000 {
		return nil, errors.New("metric cardinality limit must be between 1 and 100000")
	}
	if len(config.Views) > maxViews {
		return nil, fmt.Errorf("metric views exceed %d entries", maxViews)
	}
	options := []sdkmetric.Option{sdkmetric.WithCardinalityLimit(config.CardinalityLimit)}
	for index, view := range config.Views {
		if err := view.validate(); err != nil {
			return nil, fmt.Errorf("metric view %d: %w", index, err)
		}
		stream := sdkmetric.Stream{
			AttributeFilter: attribute.NewAllowKeysFilter(view.AllowedAttributes...),
		}
		if len(view.Boundaries) > 0 {
			stream.Aggregation = sdkmetric.AggregationExplicitBucketHistogram{
				Boundaries: append([]float64(nil), view.Boundaries...),
				NoMinMax:   view.NoMinMax,
			}
		}
		options = append(options, sdkmetric.WithView(sdkmetric.NewView(
			sdkmetric.Instrument{Name: view.Name, Unit: view.Unit},
			stream,
		)))
	}
	return options, nil
}

func (config ViewConfig) validate() error {
	var errs []error
	if len(config.AllowedAttributes) > maxAllowedAttributes {
		errs = append(errs, fmt.Errorf("allowed attributes exceed %d entries", maxAllowedAttributes))
	}
	if len(config.Boundaries) > maxBoundaries {
		errs = append(errs, fmt.Errorf("histogram boundaries exceed %d entries", maxBoundaries))
	}
	if len(config.Name) > 255 || !instrumentNamePattern.MatchString(config.Name) {
		errs = append(errs, errors.New("instrument name is invalid"))
	}
	if len(config.Unit) > 63 {
		errs = append(errs, errors.New("instrument unit exceeds 63 characters"))
	}
	if len(config.AllowedAttributes) <= maxAllowedAttributes {
		seen := make(map[attribute.Key]struct{}, len(config.AllowedAttributes))
		for _, key := range config.AllowedAttributes {
			if len(key) > 255 || !utf8.ValidString(string(key)) {
				errs = append(errs, errors.New("allowed attribute key is invalid or exceeds 255 bytes"))
				continue
			}
			if key == "" {
				errs = append(errs, errors.New("allowed attribute key cannot be empty"))
			}
			if _, duplicate := seen[key]; duplicate {
				errs = append(errs, errors.New("allowed attribute keys contain a duplicate"))
			}
			seen[key] = struct{}{}
		}
	}
	if len(config.Boundaries) <= maxBoundaries {
		for index, boundary := range config.Boundaries {
			if math.IsNaN(boundary) || math.IsInf(boundary, 0) {
				errs = append(errs, errors.New("histogram boundaries must be finite"))
			}
			if index > 0 && boundary <= config.Boundaries[index-1] {
				errs = append(errs, errors.New("histogram boundaries must be strictly increasing"))
			}
		}
	}
	return errors.Join(errs...)
}
