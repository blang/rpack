package rpack

import (
	"testing"
)

// TestValidateRPackInputs tests the ValidateRPackInputs function.
func TestValidateRPackInputs(t *testing.T) {
	tests := []struct {
		name        string
		resolved    []*RPackResolvedInput
		def         []*RPackDefInput
		expectError bool
	}{
		{
			name: "happy path",
			resolved: []*RPackResolvedInput{
				{
					Name:         "input1",
					UserPath:     "a",
					ResolvedPath: "pathA",
					Type:         RPackInputTypeFile,
				},
				{
					Name:         "input2",
					UserPath:     "b",
					ResolvedPath: "pathB",
					Type:         RPackInputTypeDirectory,
				},
			},
			def: []*RPackDefInput{
				{
					Name: "input1",
					Type: RPackDefInputTypeFile,
				},
				{
					Name: "input2",
					Type: RPackDefInputTypeDirectory,
				},
			},
			expectError: false,
		},
		{
			name: "duplicate resolved input names",
			resolved: []*RPackResolvedInput{
				{
					Name:         "input1",
					UserPath:     "a",
					ResolvedPath: "pathA",
					Type:         RPackInputTypeFile,
				},
				{
					Name:         "input1",
					UserPath:     "b",
					ResolvedPath: "pathB",
					Type:         RPackInputTypeFile,
				},
			},
			def: []*RPackDefInput{
				{
					Name: "input1",
					Type: RPackDefInputTypeFile,
				},
			},
			expectError: true,
		},
		{
			name: "duplicate def input names",
			resolved: []*RPackResolvedInput{
				{
					Name:         "input1",
					UserPath:     "a",
					ResolvedPath: "pathA",
					Type:         RPackInputTypeFile,
				},
			},
			def: []*RPackDefInput{
				{
					Name: "input1",
					Type: RPackDefInputTypeFile,
				},
				{
					Name: "input1",
					Type: RPackDefInputTypeFile,
				},
			},
			expectError: true,
		},
		{
			name: "no matching def input",
			resolved: []*RPackResolvedInput{
				{
					Name:         "input1",
					UserPath:     "a",
					ResolvedPath: "pathA",
					Type:         RPackInputTypeFile,
				},
			},
			def: []*RPackDefInput{
				{
					Name: "input2",
					Type: RPackDefInputTypeFile,
				},
			},
			expectError: true,
		},
		{
			name: "type mismatch: def requires file, resolved is directory",
			resolved: []*RPackResolvedInput{
				{
					Name:         "input1",
					UserPath:     "a",
					ResolvedPath: "pathA",
					Type:         RPackInputTypeDirectory,
				},
			},
			def: []*RPackDefInput{
				{
					Name: "input1",
					Type: RPackDefInputTypeFile,
				},
			},
			expectError: true,
		},
		{
			name: "type mismatch: def requires directory, resolved is file",
			resolved: []*RPackResolvedInput{
				{
					Name:         "input1",
					UserPath:     "a",
					ResolvedPath: "pathA",
					Type:         RPackInputTypeFile,
				},
			},
			def: []*RPackDefInput{
				{
					Name: "input1",
					Type: RPackDefInputTypeDirectory,
				},
			},
			expectError: true,
		},
	}

	for _, tc := range tests {
		tc := tc // capture range variable
		t.Run(tc.name, func(t *testing.T) {
			err := ValidateRPackInputs(tc.resolved, tc.def)
			if tc.expectError && err == nil {
				t.Errorf("expected an error but got none")
			} else if !tc.expectError && err != nil {
				t.Errorf("unexpected error: %s", err)
			}
		})
	}
}
