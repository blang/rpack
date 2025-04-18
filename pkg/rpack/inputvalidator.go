package rpack

import "github.com/pkg/errors"

// Accepts a
// RPack Instance inputs: RPackInstance.ConfigInstance(RPackConfigInstance).Config(RPackConfig).Config(RPackConfigConfig).Inputs : map[string]string
// []*RPackDefInput: from RPackDef.Inputs
// Before this can happen, the RPackInstanceInputs need to point to actual absolute paths
func ValidateRPackInputs(resolvedInputs []*RPackResolvedInput, defInputs []*RPackDefInput) error {
	// Check User Inputs names are unique
	{
		visitedNames := make(map[string]struct{})
		for _, in := range resolvedInputs {
			if _, ok := visitedNames[in.Name]; ok {
				return errors.Errorf("Resolved input %s already exists", in.Name)
			}
			visitedNames[in.Name] = struct{}{}
		}
	}

	// Check Def Inputs names are unique
	{
		visitedNames := make(map[string]struct{})
		for _, in := range defInputs {
			if _, ok := visitedNames[in.Name]; ok {
				return errors.Errorf("RPackDef input %s already exists", in.Name)
			}
			visitedNames[in.Name] = struct{}{}
		}
	}

	// Check every resolved Input matches a defInput
	for _, in := range resolvedInputs {
		var matchDefInput *RPackDefInput
		for _, defIn := range defInputs {
			if in.Name == defIn.Name {
				matchDefInput = defIn
				break
			}
		}
		if matchDefInput == nil {
			return errors.Errorf("No definition found for user input %s", in.Name)
		}
		// TODO: Refactor for proper type check
		// Maybe we can use a type already existing in stdlib
		if matchDefInput.Type == RPackDefInputTypeFile && in.Type != RPackInputTypeFile {
			return errors.Errorf("Definition for user input %s requires type file, but found directory", in.Name)
		}
		if matchDefInput.Type == RPackDefInputTypeDirectory && in.Type != RPackInputTypeDirectory {
			return errors.Errorf("Definition for user input %s requires type directory, but found file", in.Name)
		}
	}

	return nil
}
