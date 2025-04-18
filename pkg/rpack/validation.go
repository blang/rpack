package rpack

import (
	"cuelang.org/go/cue"
	"cuelang.org/go/cue/cuecontext"
	"github.com/pkg/errors"
)

type SchemaValidator interface {
	Validate(x interface{}) error
}

// EmptyValidator provides no validation
type EmptyValidator struct{}

func (c *EmptyValidator) Validate(x interface{}) error {
	return nil
}

type CueValidator struct {
	Schema  cue.Value
	Context *cue.Context
}

// NewCueValidator creates a new SchemaValidator using a cuelang schema and path to validate against.
func NewCueValidator(schemaBytes []byte, path string) (*CueValidator, error) {
	ctx := cuecontext.New()
	schema := ctx.CompileBytes(schemaBytes).LookupPath(cue.ParsePath(path))
	if !schema.Exists() {
		return nil, errors.Errorf("Cue Schema %s does not exist", path)
	}

	return &CueValidator{
		Schema:  schema,
		Context: ctx,
	}, nil
}

func (c *CueValidator) Validate(x interface{}) error {
	asCue := c.Context.Encode(x)
	unified := c.Schema.Unify(asCue)
	return unified.Validate()
}
