package rpack

import (
	"os"

	"github.com/pkg/errors"
	"sigs.k8s.io/yaml"
)

func LoadRPackDef(name string) (*RPackDef, error) {
	b, err := os.ReadFile(name)
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
