package rpack

import (
	"fmt"
	"strings"

	"cuelang.org/go/cue"
	"cuelang.org/go/cue/cuecontext"
	"cuelang.org/go/cue/errors"
)

// cueSchemaRoot is the conventional root path segment used by RPack's CUE
// schemas ("#Schema"). formatCueErrors strips it from rendered paths so user
// messages reference the user-facing config field rather than CUE internals.
const cueSchemaRoot = "#Schema"

// SchemaValidator validates data against a schema.
type SchemaValidator interface {
	Validate(x any) error
}

// EmptyValidator provides no validation
type EmptyValidator struct{}

// Validate always returns nil for the empty validator.
func (c *EmptyValidator) Validate(x any) error {
	return nil
}

// CueValidator validates data using CUE schemas.
type CueValidator struct {
	Schema  cue.Value
	Context *cue.Context
}

// NewCueValidator creates a new SchemaValidator using a cuelang schema and path to validate against.
//
// A schema that fails to compile surfaces its actual parse/compile error
// (e.g. the offending line) rather than the opaque "#Schema does not exist".
func NewCueValidator(schemaBytes []byte, path string) (*CueValidator, error) {
	ctx := cuecontext.New()
	compiled := ctx.CompileBytes(schemaBytes)
	if vErr := compiled.Err(); vErr != nil {
		return nil, fmt.Errorf("compiling CUE schema: %w", vErr)
	}
	schema := compiled.LookupPath(cue.ParsePath(path))
	if !schema.Exists() {
		return nil, fmt.Errorf("cue schema path %s does not exist in compiled schema", path)
	}

	return &CueValidator{
		Schema:  schema,
		Context: ctx,
	}, nil
}

// Validate checks data against the CUE schema.
//
// On failure, the raw cuelang error (which references internal paths like
// "#Schema.field") is rewritten into a human-readable form: the leading
// "#Schema." root is stripped and the location is prefixed with "rpack.yaml:"
// so the user can see which config field failed without learning CUE internals.
func (c *CueValidator) Validate(x any) error {
	asCue := c.Context.Encode(x)
	unified := c.Schema.Unify(asCue)
	if vErr := unified.Validate(); vErr != nil {
		return formatCueErrors(vErr)
	}
	return nil
}

// formatCueErrors renders a cuelang validation error as a self-explanatory,
// human-readable message. The internal "#Schema" root path segment is stripped
// and the location is prefixed with "rpack.yaml:", turning e.g.
// "#Schema.field: conflicting values string and 42" into
// "rpack.yaml: field: conflicting values string and 42".
func formatCueErrors(vErr error) error {
	es := errors.Errors(vErr)
	if len(es) == 0 {
		return vErr
	}
	var msgs []string
	for _, e := range es {
		msg := strings.TrimSpace(e.Error())
		// Strip the internal CUE schema root from the rendered path.
		msg = strings.TrimPrefix(msg, cueSchemaRoot+".")
		msg = strings.TrimPrefix(msg, cueSchemaRoot+":")
		msg = strings.TrimSpace(msg)
		if msg == "" {
			msg = "validation failed"
		}
		msgs = append(msgs, "rpack.yaml: "+msg)
	}
	return fmt.Errorf("%s", strings.Join(msgs, "; "))
}
