// Package handler defines the public contracts for ironstate handler plugins.
package handler

// Action is the resolved operation ironstate asks a handler to describe or run.
type Action string

const (
	ActionSkip      Action = "Skip"
	ActionInstall   Action = "Install"
	ActionUninstall Action = "Uninstall"
)

// Become describes the privilege elevation requested for a handler call.
type Become struct {
	Enabled bool
	User    string
}

// Context supplies task-wide state to a handler. Flat contains the resolved
// facts, vars, package, inputs, and registry namespaces for the current task.
type Context struct {
	Flat      map[string]any
	Apply     bool
	Become    Become
	Callbacks HostCallbacks
}

// HostCallbacks are optional services provided by the ironstate host for
// handlers that need the host's template and condition implementations.
type HostCallbacks interface {
	RenderTemplate(template string, variables map[string]any) (string, error)
	EvaluateCondition(expression string, variables map[string]any) (bool, error)
	Log(message string) error
}

// ExecResult is a handler's normalized command result.
type ExecResult struct {
	RC          int
	Stdout      string
	StdoutLines []string
	Stderr      string
	StderrLines []string
	Extra       map[string]any
}

// Handler is the uniform contract for an external handler plugin.
type Handler interface {
	Test(item map[string]any, name string, ctx Context) (bool, error)
	Describe(item map[string]any, action Action, ctx Context) (string, error)
	Install(item map[string]any, name string, ctx Context) (ExecResult, error)
	Uninstall(item map[string]any, name string, ctx Context) (ExecResult, error)
}

// EmojiProvider optionally supplies the glyph used for this handler in host
// progress and result-table output. An absent or empty value uses the host's
// default glyph.
type EmojiProvider interface {
	Emoji() string
}

// RequiredToolsProvider optionally declares the executables a handler needs
// available on PATH before ironstate dispatches it. An absent or empty value
// means the handler performs its own availability checks.
type RequiredToolsProvider interface {
	RequiredTools() []string
}

// FactProducer optionally exposes a named result as a later-task fact.
type FactProducer interface {
	FactName(item map[string]any) (name string, ok bool)
}

// ScanItem is one discovered configuration object suitable for a generated
// ironstate playbook.
type ScanItem struct {
	Module string         `yaml:"-"`
	Name   string         `yaml:"name"`
	Config map[string]any `yaml:"config"`
	Tags   []string       `yaml:"tags,omitempty"`
	Role   string         `yaml:"-"`
}

// ScanCapable optionally discovers its handler's existing configuration.
type ScanCapable interface {
	ScanRole() string
	Scan(ctx Context) ([]ScanItem, error)
}
