package rpack

import (
	"fmt"

	"cuelang.org/go/cue"
	"cuelang.org/go/cue/cuecontext"
)

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
func NewCueValidator(schemaBytes []byte, path string) (*CueValidator, error) {
	ctx := cuecontext.New()
	schema := ctx.CompileBytes(schemaBytes).LookupPath(cue.ParsePath(path))
	if !schema.Exists() {
		return nil, fmt.Errorf("cue Schema %s does not exist", path)
	}

	return &CueValidator{
		Schema:  schema,
		Context: ctx,
	}, nil
}

// Validate checks data against the CUE schema.
func (c *CueValidator) Validate(x any) error {
	asCue := c.Context.Encode(x)
	unified := c.Schema.Unify(asCue)
	return unified.Validate()
}
