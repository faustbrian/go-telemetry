package telemetryservice

import service "github.com/faustbrian/go-service/v2"

// ComponentV2 returns Service v2 hooks for this adapter's existing runtime.
// It preserves Component's initialization policy, provider ownership and shared
// one-shot shutdown result. These are alternative bindings, not separate
// lifecycles: register one component and do not start this adapter again.
func (adapter *Adapter) ComponentV2() service.Component {
	return service.Component(adapter.Component())
}
