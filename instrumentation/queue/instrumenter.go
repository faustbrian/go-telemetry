// Package telemetryqueue provides dependency-neutral queue instrumentation.
//
// It is the target-oriented successor to instrumentation/goqueue and preserves
// that package's released type and instrumentation identities.
package telemetryqueue

import (
	"context"

	goqueue "github.com/faustbrian/go-telemetry/instrumentation/goqueue"
)

type Backend = goqueue.Backend

const (
	BackendMemory       = goqueue.BackendMemory
	BackendRedis        = goqueue.BackendRedis
	BackendRedisStream  = goqueue.BackendRedisStream
	BackendValkeyStream = goqueue.BackendValkeyStream
	BackendNATS         = goqueue.BackendNATS
	BackendNSQ          = goqueue.BackendNSQ
	BackendRabbitMQ     = goqueue.BackendRabbitMQ
	BackendOther        = goqueue.BackendOther
)

type Config = goqueue.Config
type Instrumenter = goqueue.Instrumenter

func New(config Config) (*Instrumenter, error) {
	return goqueue.New(config)
}

func WrapHandler[Message any](
	instrumenter *Instrumenter,
	handler func(context.Context, Message) error,
) func(context.Context, Message) error {
	return goqueue.WrapHandler(instrumenter, handler)
}
