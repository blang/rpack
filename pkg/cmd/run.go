// Package cmd implements the run command.
package cmd

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/spf13/cobra"

	"github.com/blang/rpack/pkg/rpack"
)

// runCmd represents the run command
var runCmd = &cobra.Command{
	Use:   "run [--def <dir>] [flags] [<config-file>]",
	Short: "Run an rpack file or definition directory",
	Args:  cobra.MaximumNArgs(1),
	Long: `Execute an rpack from a user config file or a local definition directory.

With a config file:
  rpack run ./app.rpack.yaml

With a local definition directory (--def mode):
  rpack run --def ./my-rpack --set author=test --dry-run`,
	RunE: func(cmd *cobra.Command, args []string) error {
		defDir, err := cmd.Flags().GetString("def")
		if err != nil {
			return err
		}
		hasConfigFile := len(args) > 0

		// Validate flag combinations
		if defDir != "" && hasConfigFile {
			return fmt.Errorf("--def and config file argument are mutually exclusive")
		}
		if defDir == "" && !hasConfigFile {
			return fmt.Errorf("either --def or a config file argument is required")
		}

		// Parse --set flags (only valid with --def)
		setFlags, err := cmd.Flags().GetStringSlice("set")
		if err != nil {
			return err
		}
		if len(setFlags) > 0 && defDir == "" {
			return fmt.Errorf("--set requires --def")
		}

		// Parse --set-input flags (only valid with --def)
		setInputFlags, err := cmd.Flags().GetStringSlice("set-input")
		if err != nil {
			return err
		}
		if len(setInputFlags) > 0 && defDir == "" {
			return fmt.Errorf("--set-input requires --def")
		}

		// Parse --output-dir
		outputDir, err := cmd.Flags().GetString("output-dir")
		if err != nil {
			return err
		}

		// --output-dir and --dry-run are mutually exclusive
		flagDryRun, err := cmd.Flags().GetBool("dry-run")
		if err != nil {
			return err
		}
		if outputDir != "" && flagDryRun {
			return fmt.Errorf("--output-dir and --dry-run are mutually exclusive")
		}

		e := &rpack.Executor{}

		flagWD, err := cmd.Flags().GetString("working-dir")
		if err != nil {
			return err
		}
		if flagWD != "" {
			e.OverrideExecPath = flagWD
		}

		flagForce, err := cmd.Flags().GetBool("force")
		if err != nil {
			return err
		}
		e.Force = flagForce

		e.DryRun = flagDryRun
		e.OutputDir = outputDir

		if defDir != "" {
			// --def mode
			values, err := parseSetFlags(setFlags)
			if err != nil {
				return fmt.Errorf("invalid --set flag: %w", err)
			}

			inputs, err := parseSetInputFlags(setInputFlags)
			if err != nil {
				return fmt.Errorf("invalid --set-input flag: %w", err)
			}

			return e.ExecRPackDirect(context.TODO(), defDir, values, inputs)
		}

		// Normal mode (config file)
		if err := e.ExecRPack(context.TODO(), args[0]); err != nil {
			return err
		}
		return nil
	},
}

func init() {
	rootCmd.AddCommand(runCmd)

	runCmd.Flags().StringP("def", "", "", "Use local definition directory (mutually exclusive with config file)")
	runCmd.Flags().StringSliceP("set", "", nil, "Set a config value (key=value, repeatable)")
	runCmd.Flags().StringSliceP("set-input", "", nil, "Map an input name to a local file (name=path, repeatable)")
	runCmd.Flags().StringP("output-dir", "", "", "Write output files to this directory")
	runCmd.Flags().StringP("working-dir", "w", "", "Override working dir, defaults to location of rpack file")
	runCmd.Flags().BoolP("force", "f", false, "Force execution: Overwrite files, ignore warnings")
	runCmd.Flags().BoolP("dry-run", "", false, "Dry run execution")
}

// parseSetFlags parses --set key=value flags into a map[string]any.
// Supports type coercion (int, bool, float, string), dot-notation nesting,
// and array indexing.
//
// Array semantics: a key that appears multiple times produces a list.
// A single occurrence produces a scalar (string/int/bool/float).
// Index notation (key.0, key.1) always produces a list.
// Mixing index and duplicate-key on the same key is an error.
func parseSetFlags(raw []string) (map[string]any, error) {
	result := make(map[string]any)

	for _, rawFlag := range raw {
		key, value, ok := strings.Cut(rawFlag, "=")
		if !ok {
			return nil, fmt.Errorf("invalid format %q, expected key=value", rawFlag)
		}

		parsed := coerceValue(value)
		if err := setNestedValue(result, key, parsed); err != nil {
			return nil, err
		}
	}

	return result, nil
}

// parseSetInputFlags parses --set-input name=path flags into a map[string]string.
func parseSetInputFlags(raw []string) (map[string]string, error) {
	result := make(map[string]string)
	for _, rawFlag := range raw {
		name, path, ok := strings.Cut(rawFlag, "=")
		if !ok {
			return nil, fmt.Errorf("invalid format %q, expected name=path", rawFlag)
		}

		// Verify the file/dir exists
		if _, err := os.Stat(path); err != nil {
			return nil, fmt.Errorf("input path %q does not exist: %w", path, err)
		}
		result[name] = path
	}
	return result, nil
}

// coerceValue auto-detects the type of a string value.
func coerceValue(s string) any {
	if s == "true" {
		return true
	}
	if s == "false" {
		return false
	}
	if i, err := strconv.Atoi(s); err == nil {
		return i
	}
	if f, err := strconv.ParseFloat(s, 64); err == nil {
		return f
	}
	return s
}

// setNestedValue sets a value in a nested map, supporting dot-notation
// and array index notation (key.0, key.1).
func setNestedValue(m map[string]any, key string, value any) error {
	parts := strings.Split(key, ".")
	if len(parts) == 0 {
		return fmt.Errorf("empty key")
	}

	// Walk to the second-to-last part, creating maps as needed.
	current := m
	for i := 0; i < len(parts)-1; i++ {
		part := parts[i]

		// Check if this part is an array index.
		if idx, err := strconv.Atoi(part); err == nil && fmt.Sprintf("%d", idx) == part {
			// This is an array index — the parent must be an array.
			// Walk back to find the parent key.
			return setNestedValueWithArray(m, parts, value, i)
		}

		next, ok := current[part]
		if !ok {
			newMap := make(map[string]any)
			current[part] = newMap
			current = newMap
			continue
		}

		switch v := next.(type) {
		case map[string]any:
			current = v
		case []any:
			// Existing value is an array. If the next parts[i+1] is an
			// integer index, delegate to the array handler.
			if i+1 < len(parts) {
				if _, err := strconv.Atoi(parts[i+1]); err == nil {
					return setNestedValueWithArray(m, parts, value, i+1)
				}
			}
			return fmt.Errorf("cannot nest under array key %q", part)
		default:
			return fmt.Errorf("cannot nest under non-object key %q (type %T)", part, next)
		}
	}

	// Set the final value. Check for duplicate key → array behavior.
	lastPart := parts[len(parts)-1]

	// If this is an index (e.g., "0"), handle it as array.
	if idx, err := strconv.Atoi(lastPart); err == nil && fmt.Sprintf("%d", idx) == lastPart {
		return setNestedValueWithArray(m, parts, value, len(parts)-1)
	}

	existing, exists := current[lastPart]
	if !exists {
		current[lastPart] = value
		return nil
	}

	// Key already exists. If it's already a slice, append. Otherwise, convert to slice.
	switch existingVal := existing.(type) {
	case []any:
		current[lastPart] = append(existingVal, value)
	default:
		// Convert single value to list. But check for mixed index+duplicate.
		// If the key contains an index anywhere in the key path, it's ambiguous.
		current[lastPart] = []any{existingVal, value}
	}

	return nil
}

// setNestedValueWithArray handles setting a value at an indexed position
// within nested arrays and maps.
// parts is the full key path, arrayIdx is the position of the numeric index in parts.
// For "list.0", parts=["list","0"], arrayIdx=1.
// For "nested.list.0.name", parts=["nested","list","0","name"], arrayIdx=2.
func setNestedValueWithArray(m map[string]any, parts []string, value any, arrayIdx int) error {
	if arrayIdx == 0 {
		return fmt.Errorf("array index at root level is not supported: %q", strings.Join(parts, "."))
	}

	arrayKey := parts[arrayIdx-1] // the key that holds the array

	// Walk to the parent map that contains the array key.
	current := m
	for i := 0; i < arrayIdx-1; i++ {
		part := parts[i]
		next, ok := current[part]
		if !ok {
			newMap := make(map[string]any)
			current[part] = newMap
			current = newMap
			continue
		}
		switch v := next.(type) {
		case map[string]any:
			current = v
		default:
			return fmt.Errorf("cannot index into non-object at key %q", part)
		}
	}

	arrIdxInt, err := strconv.Atoi(parts[arrayIdx])
	if err != nil {
		return fmt.Errorf("expected array index, got %q", parts[arrayIdx])
	}

	// Get or create the array.
	var arr []any
	existing := current[arrayKey]
	switch v := existing.(type) {
	case []any:
		arr = v
	case map[string]any:
		// Empty map from eager walk by setNestedValue — treat as nil and create array.
		// Non-empty map means previous --set call set a nested object, which is a conflict.
		if len(v) > 0 {
			return fmt.Errorf("cannot use index notation on key %q: already set as a nested object", arrayKey)
		}
		arr = make([]any, arrIdxInt+1)
	case nil:
		arr = make([]any, arrIdxInt+1)
	default:
		return fmt.Errorf("key %q is not an array (type %T), cannot index into it", arrayKey, existing)
	}

	// Ensure array is large enough.
	for len(arr) <= arrIdxInt {
		arr = append(arr, nil)
	}

	remaining := parts[arrayIdx+1:] // parts after the index

	// If there are no more parts after the index, set the value directly.
	if len(remaining) == 0 {
		arr[arrIdxInt] = value
		current[arrayKey] = arr
		return nil
	}

	// There are more parts — the array element should be a map.
	var elemMap map[string]any
	if arr[arrIdxInt] == nil {
		elemMap = make(map[string]any)
		arr[arrIdxInt] = elemMap
	} else {
		var ok bool
		elemMap, ok = arr[arrIdxInt].(map[string]any)
		if !ok {
			return fmt.Errorf("array element at %s[%d] is not an object (type %T)", arrayKey, arrIdxInt, arr[arrIdxInt])
		}
	}

	// Recurse into the nested map.
	subKey := strings.Join(remaining, ".")
	if err := setNestedValue(elemMap, subKey, value); err != nil {
		return err
	}
	current[arrayKey] = arr
	return nil
}
