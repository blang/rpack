package rpack

import (
	"os"
	"path/filepath"

	"log/slog"

	"github.com/blang/rpack/pkg/rpack/util"
	"github.com/hashicorp/go-getter"
	"github.com/pkg/errors"
)

// RPackInstance is an executable instance of rpack
type RPackInstance struct {
	// Absolute path to where to execute the rpack
	ExecPath string

	ConfigInstance *RPackConfigInstance

	// Root path of cache instance for this rpack
	CachePath string

	// Temp path of cache instance for this rpack, used to write temporary files
	TempPath string

	// RunPath is the directory target files are written to.
	RunPath string

	// SourcePath containing the downloaded source
	SourcePath string

	// All user specified inputs resolved to point to actual files
	ResolvedInputs []*RPackResolvedInput
}

type RPackInputType string

const (
	RPackInputTypeFile      RPackInputType = "file"
	RPackInputTypeDirectory RPackInputType = "dir"
)

type RPackResolvedInput struct {
	Name string

	// Cleaned up user path, is relative and local
	UserPath string

	ResolvedPath string
	Type         RPackInputType
}

// ResolveRPackInputs resolves the user provided inputs in the context of an execution path
// to actual files and directories on disk.
// It checks if the type specified by the RPackDef is matching against the supplied type.
func ResolveRPackInputs(configInputs map[string]string, execPath string) ([]*RPackResolvedInput, error) {
	var resolvedInputs []*RPackResolvedInput
	for name, userPath := range configInputs {
		cleanUserPath := filepath.Clean(userPath)
		// Check path boundaries
		if filepath.IsAbs(cleanUserPath) {
			return nil, errors.Errorf("User path %s=%s is not relative", name, userPath)
		}
		if !filepath.IsLocal(cleanUserPath) {
			return nil, errors.Errorf("User path %s=%s is not local", name, userPath)
		}

		absPath := filepath.Join(execPath, cleanUserPath)
		absPath = filepath.Clean(absPath)

		isDir, err := util.CheckFileOrDirExists(absPath)
		if err != nil {
			return nil, errors.Wrapf(err, "User path %s=%s does not exist", name, userPath)
		}
		fileType := RPackInputTypeFile
		if isDir {
			fileType = RPackInputTypeDirectory
		}
		resolvedInputs = append(resolvedInputs, &RPackResolvedInput{
			Name:         name,
			UserPath:     cleanUserPath,
			ResolvedPath: absPath,
			Type:         fileType,
		})
	}
	return resolvedInputs, nil
}

const (
	RPackCacheDir       = ".rpack.d"
	RPackCacheDirSource = "source"
	RPackCacheDirRun    = "run"
	RPackCacheDirTemp   = "tmp"
)

// LoadRPack loads all required data of a RPack to be executed.
func LoadRPack(ci *RPackConfigInstance, execPath string) (*RPackInstance, error) {

	// Setup cache path
	packCachePath := filepath.Join(execPath, RPackCacheDir, util.Sha256String(ci.Config.Source))
	err := os.MkdirAll(packCachePath, 0755)
	if err != nil {
		return nil, errors.Wrapf(err, "Could not setup cache path %s", packCachePath)
	}

	// Setup source path
	packSourcePath := filepath.Join(packCachePath, RPackCacheDirSource)
	// Do not create last part of path, since go-getter is required to create it , since it creates symlinks for local references
	err = os.MkdirAll(filepath.Dir(packSourcePath), 0755)
	if err != nil {
		return nil, errors.Wrapf(err, "Could not setup source path %s", packSourcePath)
	}

	// Setup run path
	shaConfigPath := util.Sha256String(ci.ConfigPath)
	packRunPath := filepath.Join(packCachePath, shaConfigPath, RPackCacheDirRun)
	// Cleanup RunPath first
	if _, err := os.Stat(packRunPath); err == nil {
		err = os.RemoveAll(packRunPath)
		if err != nil {
			return nil, errors.Wrapf(err, "Could not cleanup run path: %s", packRunPath)
		}
	}
	err = os.MkdirAll(packRunPath, 0755)
	if err != nil {
		return nil, errors.Wrapf(err, "Could not setup run path %s", packRunPath)
	}

	// Setup tmp path
	packTempPath := filepath.Join(packCachePath, shaConfigPath, RPackCacheDirTemp)
	// Cleanup TempPath first
	if _, err := os.Stat(packTempPath); err == nil {
		err = os.RemoveAll(packTempPath)
		if err != nil {
			return nil, errors.Wrapf(err, "Could not cleanup temp path: %s", packTempPath)
		}
	}
	err = os.MkdirAll(packTempPath, 0755)
	if err != nil {
		return nil, errors.Wrapf(err, "Could not setup temp path %s", packTempPath)
	}

	slog.Debug("Load RPackDef", "source", packSourcePath, "dest", ci.Config.Source)
	// Load RPackDef into source folder
	client := &getter.Client{
		Src:     ci.Config.Source,
		Dst:     packSourcePath,
		Dir:     true,
		Options: []getter.ClientOption{getter.WithMode(getter.ClientModeDir)},
		Pwd:     execPath,
	}
	err = client.Get()
	if err != nil {
		return nil, errors.Wrapf(err, "Could not get source %q", ci.Config.Source)
	}

	// TODO: Should we load the RPackDef here too?

	// Resolve user specified inputs
	resolvedInputs, err := ResolveRPackInputs(ci.Config.Config.Inputs, execPath)
	if err != nil {
		return nil, errors.Wrap(err, "Could not resolve user inputs")
	}

	return &RPackInstance{
		ConfigInstance: ci,
		ExecPath:       execPath,
		CachePath:      packCachePath,
		TempPath:       packTempPath,
		RunPath:        packRunPath,
		SourcePath:     packSourcePath,
		ResolvedInputs: resolvedInputs,
	}, nil
}

const (
	RPackDefDefaultFilename = "rpack.yaml"
	RPackDefSchemaFilename  = "schema.cue"
	RPackDefScriptFilename  = "script.lua"
)

// RPackDefInstance contains a prepared execution environment
// of a RPackDef.
type RPackDefInstance struct {
	// Source directory where rpack.yaml is stored
	Source string

	// Absolute path to the script
	ScriptPath string

	// Deserialized RPackDef rpack.yaml
	Def *RPackDef

	// Validate user values and inputs
	ConfigValidator SchemaValidator
}

// ValidateConfig validates the values and inputs of a RPack against the schema of a RPackDef.
func (i *RPackDefInstance) ValidateConfig(c *RPackConfig) error {
	if err := i.ConfigValidator.Validate(c.Config); err != nil {
		return errors.Wrap(err, "Validation of config failed")
	}
	return nil
}

// SetupRPackDefInstance loads the RPackDef from the given source path
// and sets up the RPackDefInstance for validation and execution.
func SetupRPackDefInstance(source string) (*RPackDefInstance, error) {

	// LoadDefinition
	defPath := filepath.Join(source, RPackDefDefaultFilename)
	def, err := LoadRPackDef(defPath)
	if err != nil {
		return nil, errors.Wrapf(err, "Could not load RPack definition file %s", defPath)
	}

	// Validate Definition
	err = def.ValidateSchema()
	if err != nil {
		return nil, errors.Wrapf(err, "Defintion schema validation failed: %s", defPath)
	}

	var vc SchemaValidator
	// Load optional value schema file in cuelang
	schemaFile := filepath.Join(source, RPackDefSchemaFilename)
	if _, err := os.Stat(schemaFile); err == nil { // File exists
		// Parse schema and validate values

		b, err := os.ReadFile(schemaFile)
		if err != nil {
			return nil, errors.Wrapf(err, "Failed to open schema file: %s", schemaFile)
		}

		vc, err = NewCueValidator(b, RPackDefSchemaName)
		if err != nil {
			return nil, errors.Wrapf(err, "Could not create validation context from path %s in schema file %s", RPackDefSchemaName, schemaFile)
		}
	} else {
		vc = &EmptyValidator{}
	}

	// Check script
	scriptPath := filepath.Join(source, RPackDefScriptFilename)
	if _, err := os.Stat(scriptPath); err != nil {
		return nil, errors.Wrapf(err, "Could not access script file: %s", scriptPath)
	}

	return &RPackDefInstance{
		Source:          source,
		Def:             def,
		ConfigValidator: vc,
		ScriptPath:      scriptPath,
	}, nil
}
