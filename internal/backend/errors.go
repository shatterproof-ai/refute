package backend

import "fmt"

// ErrAdapterRuntimeMissing signals that a subprocess rewrite adapter's runtime
// dependency is absent — for example the ts-morph Node modules are not
// installed, or the OpenRewrite adapter JAR has not been built. It is distinct
// from a missing LSP server (handled in the CLI layer) and from an unsupported
// operation: the operation is supported, but the tool needed to run it is not
// present.
//
// It is returned by an adapter's Initialize (or first subprocess invocation) so
// the CLI can tell the user to install the runtime rather than reporting a
// generic "backend failed" error. The InstallHint field carries the next action
// when a provisioning command is known.
type ErrAdapterRuntimeMissing struct {
	// Language is the language the adapter serves, e.g. "typescript", "java".
	Language string
	// AdapterName identifies the adapter, e.g. "ts-morph", "openrewrite".
	AdapterName string
	// MissingRuntime describes what is absent, e.g. "ts-morph node modules",
	// "OpenRewrite adapter JAR".
	MissingRuntime string
	// InstallHint is the command or next action that provisions the runtime,
	// when known.
	InstallHint string
}

func (e *ErrAdapterRuntimeMissing) Error() string {
	msg := fmt.Sprintf("%s adapter runtime missing", e.AdapterName)
	if e.MissingRuntime != "" {
		msg += ": " + e.MissingRuntime
	}
	if e.InstallHint != "" {
		msg += "; install with: " + e.InstallHint
	}
	return msg
}
