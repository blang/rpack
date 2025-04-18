package rpack

import (
	_ "embed"

	"github.com/pkg/errors"
	"github.com/samber/lo"
)

//go:embed def_schema.cue
var RPackDefSchema string

const (
	RPackDefSchemaName         = "#Schema"
	RPackDefInternalSchemaName = "#Schema"
)

// RPackDef is the definition of a rpack represented by the rpack.yaml
type RPackDef struct {
	SchemaVersion string `json:"@schema_version"`

	// Name of definition, required
	Name string `json:"name"`

	// ScriptFile to execute: default: script.lua
	// ScriptFile string     `json:"script_file"`

	// ConfigSchemaFile: default: schema.cue

	// Inputs define paths (files and dirs) that can be read outside the rpack
	// definition that are mapped by the user.
	// Those paths are excluded from write operations.
	Inputs []*RPackDefInput `json:"inputs"`
}

var RPackDefSchemaValidator = lo.Must(NewCueValidator([]byte(RPackDefSchema), RPackDefInternalSchemaName))

// TODO: Can use a SchemaValidator
func (def *RPackDef) ValidateSchema() error {
	err := RPackDefSchemaValidator.Validate(def)
	if err != nil {
		return errors.Wrap(err, "Validating rpack definition failed")
	}
	return nil
}

// TODO: Make this an enum type, but also requires ability in json unmarshaller
const (
	RPackDefInputTypeFile      = "file"
	RPackDefInputTypeDirectory = "dir"
)

// RPackDefInput defines a potential input for the rpack.
type RPackDefInput struct {
	// Type: dir or file
	Type string `json:"type"`

	// Name to reference path in script
	Name string `json:"name"`

	// // If the input is required
	// Required bool `json:"required"`
}
