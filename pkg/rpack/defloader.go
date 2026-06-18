package rpack

import (
	"os"

	"fmt"

	"sigs.k8s.io/yaml"
)

// LoadRPackDef loads an rpack definition from the given path.
func LoadRPackDef(name string) (*RPackDef, error) {
	b, err := os.ReadFile(name) //nolint:gosec // intentional: path comes from user config
	if err != nil {
		return nil, fmt.Errorf("failed to open file: %s: %w", name, err)
	}
	var c RPackDef
	err = yaml.Unmarshal(b, &c)
	if err != nil {
		return nil, fmt.Errorf("failed to unmarshal yaml in file: %s: %w", name, err)
	}
	return &c, nil
}
