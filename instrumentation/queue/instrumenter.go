// Package telemetryqueue provides dependency-neutral queue instrumentation.
//
// It is the target-oriented successor to instrumentation/goqueue and preserves
// that package's released type and instrumentation identities.
package telemetryqueue

import (
	"context"

	goqueue "github.com/faustbrian/go-telemetry/instrumentation/goqueue"
)

// Backend is a finite queue backend label.
type Backend = goqueue.Backend

const (
	// BackendMemory identifies the in-memory queue.
	BackendMemory = goqueue.BackendMemory
	// BackendRedis identifies the Redis list queue.
	BackendRedis = goqueue.BackendRedis
	// BackendRedisStream identifies Redis Streams.
	BackendRedisStream = goqueue.BackendRedisStream
	// BackendValkeyStream identifies Valkey Streams.
	BackendValkeyStream = goqueue.BackendValkeyStream
	// BackendNATS identifies NATS.
	BackendNATS = goqueue.BackendNATS
	// BackendNSQ identifies NSQ.
	BackendNSQ = goqueue.BackendNSQ
	// BackendRabbitMQ identifies RabbitMQ.
	BackendRabbitMQ = goqueue.BackendRabbitMQ
	// BackendOther identifies an explicitly supported custom backend.
	BackendOther = goqueue.BackendOther
)

// Config selects the fixed backend and standard providers.
type Config = goqueue.Config

// Instrumenter owns bounded queue handler instruments.
type Instrumenter = goqueue.Instrumenter

// New constructs a dependency-neutral queue instrumenter.
func New(config Config) (*Instrumenter, error) {
	return goqueue.New(config)
}

// WrapHandler instruments a queue-compatible handler signature.
func WrapHandler[Message any](
	instrumenter *Instrumenter,
	handler func(context.Context, Message) error,
) func(context.Context, Message) error {
	return goqueue.WrapHandler(instrumenter, handler)
}
