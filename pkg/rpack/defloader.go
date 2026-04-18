package rpack

import (
	"os"

	"github.com/pkg/errors"
	"sigs.k8s.io/yaml"
)

// LoadRPackDef loads an rpack definition from the given path.
func LoadRPackDef(name string) (*RPackDef, error) {
	b, err := os.ReadFile(name) //nolint:gosec // intentional: path comes from user config
	if err != nil {
		return nil, errors.Wrapf(err, "Failed to open file: %s", name)
	}
	var c RPackDef
	err = yaml.Unmarshal(b, &c)
	if err != nil {
		return nil, errors.Wrapf(err, "Failed to unmarshal yaml in file: %s", name)
	}
	return &c, nil
}
