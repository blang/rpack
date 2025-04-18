package rpack

import "testing"

func TestRPackDefValidateSchema(t *testing.T) {
	tcs := []struct {
		def   *RPackDef
		valid bool
	}{
		{
			def:   &RPackDef{},
			valid: false,
		},
		{
			def: &RPackDef{
				SchemaVersion: "v1",
			},
			valid: false,
		},
		{ // Minimal working
			def: &RPackDef{
				SchemaVersion: "v1",
				Name:          "name",
			},
			valid: true,
		},
		{ // With inputs
			def: &RPackDef{
				SchemaVersion: "v1",
				Name:          "name",
				Inputs: []*RPackDefInput{
					{
						Type: "file",
						Name: "name",
						// Required: false,
					},
					{
						Type: "dir",
						Name: "dirname",
						// Required: false,
					},
				},
			},
			valid: true,
		},
		{ // With invalid file types
			def: &RPackDef{
				SchemaVersion: "v1",
				Name:          "name",
				Inputs: []*RPackDefInput{
					{
						Type: "files", // should be file
						Name: "name",
						// Required: false,
					},
				},
			},
			valid: false,
		},
	}

	for i, tc := range tcs {
		err := tc.def.ValidateSchema()
		if err != nil {
			if tc.valid {
				t.Errorf("Testcase %d: Failed to validate schema: %s", i+1, err)
			}
		} else {
			if !tc.valid {
				t.Errorf("Testcase %d: Schema validated, but should fail", i+1)
			}
		}
	}
}
