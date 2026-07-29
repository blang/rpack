package rpack

import (
	"fmt"
	"os"
	"strings"

	"sigs.k8s.io/yaml"
)

// LoadRPackDef loads an rpack definition from the given path.
func LoadRPackDef(name string) (*RPackDef, error) {
	b, err := os.ReadFile(name) //nolint:gosec // intentional: path comes from user config
	if err != nil {
		return nil, fmt.Errorf("failed to open file: %s: %w", name, err)
	}

	// Select the definition contract from the minimal header before decoding
	// the full document. A v1-only runtime must reject a future contract rather
	// than trying to interpret its fields using the v1 Go type.
	var header struct {
		SchemaVersion string `json:"@schema_version"`
	}
	if err = yaml.Unmarshal(b, &header); err != nil {
		return nil, fmt.Errorf("failed to read definition contract in file %s: %w", name, err)
	}
	if header.SchemaVersion == "" {
		return nil, fmt.Errorf(
			"invalid definition file %s: rpack definition contract version is required (supported: %s)",
			name,
			strings.Join(supportedDefinitionContractVersions(), ", "),
		)
	}
	if _, err = definitionContractFor(header.SchemaVersion); err != nil {
		return nil, fmt.Errorf("invalid definition file %s: %w", name, err)
	}

	var c RPackDef
	if err = yaml.UnmarshalStrict(b, &c); err != nil {
		return nil, fmt.Errorf("failed to unmarshal yaml in file %s: %w", name, err)
	}
	return &c, nil
}
