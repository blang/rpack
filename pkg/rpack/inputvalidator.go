package rpack

import "fmt"

// ValidateRPackInputs validates the inputs for an rpack configuration.
// Accepts a
// RPack Instance inputs: RPackInstance.ConfigInstance(RPackConfigInstance).Config(RPackConfig).Config(RPackConfigConfig).Inputs : map[string]string
// []*RPackDefInput: from RPackDef.Inputs
// Before this can happen, the RPackInstanceInputs need to point to actual absolute paths
func ValidateRPackInputs(resolvedInputs []*RPackResolvedInput, defInputs []*RPackDefInput) error {
	// Check User Inputs names are unique
	resolvedByName := make(map[string]*RPackResolvedInput, len(resolvedInputs))
	for _, in := range resolvedInputs {
		if _, ok := resolvedByName[in.Name]; ok {
			return fmt.Errorf("resolved input %s already exists", in.Name)
		}
		resolvedByName[in.Name] = in
	}

	// Check Def Inputs names are unique
	defByName := make(map[string]*RPackDefInput, len(defInputs))
	for _, in := range defInputs {
		if _, ok := defByName[in.Name]; ok {
			return fmt.Errorf("rpackdef input %s already exists", in.Name)
		}
		defByName[in.Name] = in
	}

	// Check every explicitly required definition input was supplied. Inputs
	// without required set retain the legacy optional-by-default behavior.
	for _, in := range defInputs {
		if _, ok := resolvedByName[in.Name]; in.Required && !ok {
			return fmt.Errorf("required input %q was not provided: %w", in.Name, ErrInputValidation)
		}
	}

	// Check every resolved Input matches a defInput
	for _, in := range resolvedInputs {
		matchDefInput, ok := defByName[in.Name]
		if !ok {
			return fmt.Errorf("no definition found for user input %s: %w", in.Name, ErrInputValidation)
		}
		// TODO: Refactor for proper type check
		// Maybe we can use a type already existing in stdlib
		if matchDefInput.Type == RPackDefInputTypeFile && in.Type != RPackInputTypeFile {
			return fmt.Errorf("definition for user input %s requires type file, but found directory: %w", in.Name, ErrInputValidation)
		}
		if matchDefInput.Type == RPackDefInputTypeDirectory && in.Type != RPackInputTypeDirectory {
			return fmt.Errorf("definition for user input %s requires type directory, but found file: %w", in.Name, ErrInputValidation)
		}
	}

	return nil
}
