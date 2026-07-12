package config

import "fmt"

// ConfigError reports a failure loading or building a pipeline definition. It
// names the stage and field where known, and unwraps to the underlying cause
// (a decode error, a *pipe.StageError, or a pipeline construction error).
type ConfigError struct {
	Stage string // "" when not stage-scoped (e.g. a decode error)
	Field string // "" when not field-scoped
	Cause error
}

// Error renders `config: [stage "s"] [field "f"]: <cause>`, including whichever
// of stage/field are set.
func (e *ConfigError) Error() string {
	switch {
	case e.Stage != "" && e.Field != "":
		return fmt.Sprintf("config: stage %q field %q: %v", e.Stage, e.Field, e.Cause)
	case e.Stage != "":
		return fmt.Sprintf("config: stage %q: %v", e.Stage, e.Cause)
	case e.Field != "":
		return fmt.Sprintf("config: field %q: %v", e.Field, e.Cause)
	default:
		return fmt.Sprintf("config: %v", e.Cause)
	}
}

// Unwrap returns the underlying cause (a decode, stage, expr, or pipeline error)
// for errors.Is/As.
func (e *ConfigError) Unwrap() error { return e.Cause }
