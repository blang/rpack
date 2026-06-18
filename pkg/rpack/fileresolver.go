package rpack

import (
	"path/filepath"
	"strings"

	"fmt"

	"github.com/blang/rpack/pkg/rpack/util"
)

// FileResolverLocation represents the location type for file resolution.
type FileResolverLocation string

// File resolver location types.
const (
	FileResolverLocationRPack FileResolverLocation = "rpack"

	// FileResolverLocationSource specifies the execution path, the source of the rpack (not definition).
	FileResolverLocationSource FileResolverLocation = "source"
	FileResolverLocationTemp   FileResolverLocation = "temp"
	FileResolverLocationMapped FileResolverLocation = "map"
)

// ControlledFile wraps a file with access control.
type ControlledFile struct {
	// Name of the Map if it is available
	MapName string

	// Relative path to the file in the context of the location
	Path string

	// Absolute path to the file
	AbsPath string

	// Location of the file
	Location FileResolverLocation
}

// FileResolver resolves file paths. DEPRECATED: Use FS instead.
// FileResolver resolves files in a rpack script file to real files.
// The RPackDef script will use certain functions to read and write files.
// This file access needs to be sandboxed in well confined space and mapped to different
// directories depending on the purpose (the RPack Def directory, the users execution path, a temp dir,
// the actual output/run directory).
// The resolver does not check for the existence of the specified files.
//
// There are some rules that apply to all file access, if input or output:
// - Path needs to be relative
// - Path is not allowed to contain indirections such as ../ and is not allowed to leave specified base.
// In go terms the path is local to a base path.
//
// The paths are resolved as follows:
//
// Input paths:
// map:my-mapping-name          -> Resolved Input specified in the RPackDef and mapped by the user in the RPack file, can be a dir or file
// map:my-mapping-dir/dir/file  -> Resolved Input specified as directory in the RPackDef
// rpack:./my-file, rpack:my-dir/my-file -> File from RPackDef checked out source.
// temp:./myfile -> File to a temp directory
//
// Output paths:
// temp:./myfile -> file to a temp directory
// ./dir/file -> file mapped to run path
//
// Special condition:
// Even if the user specified a mapping to a specific file, it is not allowed to write to a file that was read before.
// This would result in an RPack execution that is not pure (can have side-effects and is not idempotent).
type FileResolver struct {
	// Directory of the definition itself.
	// Thats the directory that contains the rpack.yaml file, so the cloned definition.
	defSourcePath string

	// Empty directory reserved for the output of this rpack run
	runPath string

	// Temp directory reserved for temporary file access
	tempPath string

	// Path the rpack is executed in.
	// Should not be modified in any way directly, but file access is redirected to runPath.
	// TODO: Probably not used and replaced with RPackResolvedInputs?
	execPath string

	// Resolved inputs from rpack def
	resolvedInputs []*RPackResolvedInput
}

// NewFileResolver creates a new file resolver. DEPRECATED: Use NewFileResolverFS instead.
// TODO: Needs better constructor, potential problem of mixing paths.
func NewFileResolver(defSourcePath, runPath, tempPath, execPath string, resolvedInputs []*RPackResolvedInput) (*FileResolver, error) {
	ensureDir := func(path string) error {
		if dir, err := util.CheckFileOrDirExists(path); err != nil {
			return fmt.Errorf("failed to use path: %w", err)
		} else if !dir {
			return fmt.Errorf("not a directory")
		}
		return nil
	}

	if err := ensureDir(defSourcePath); err != nil {
		return nil, fmt.Errorf("failed to use defSourcePath: %s: %w", defSourcePath, err)
	}
	if err := ensureDir(runPath); err != nil {
		return nil, fmt.Errorf("failed to use runPath: %s: %w", runPath, err)
	}
	if err := ensureDir(tempPath); err != nil {
		return nil, fmt.Errorf("failed to use tempPath: %s: %w", tempPath, err)
	}
	if err := ensureDir(execPath); err != nil {
		return nil, fmt.Errorf("failed to use execPath: %s: %w", execPath, err)
	}

	return &FileResolver{
		defSourcePath:  defSourcePath,
		runPath:        runPath,
		tempPath:       tempPath,
		execPath:       execPath,
		resolvedInputs: resolvedInputs,
	}, nil
}

// ResolveInput resolves user defined file paths from script to absolute paths mapping to different locations.
func (r *FileResolver) ResolveInput(name string) (*ControlledFile, error) {
	prefix, suffix, found := strings.Cut(name, ":")
	if !found {
		return nil, fmt.Errorf("input path needs to use map:, rpack:, or temp: prefix")
	}
	switch prefix {
	case "map":
		// Resolve map file
		return r.resolveMapInput(suffix)

	case "rpack":
		// Resolve file in rpack def source
		return r.resolveRPackPath(suffix)
	case "temp":
		// Resolve file to the temp directory
		return r.resolveTempPath(suffix)
	}
	return nil, fmt.Errorf("path prefix %q not valid in %q", prefix, name)
}

func (r *FileResolver) resolveMapInput(name string) (*ControlledFile, error) {
	prefix, suffix, found := strings.Cut(name, "/")
	// Resolve prefix first, it is always given
	var resolvedInput *RPackResolvedInput
	for _, ri := range r.resolvedInputs {
		if ri.Name == prefix {
			resolvedInput = ri
			break
		}
	}
	if resolvedInput == nil {
		return nil, fmt.Errorf("could not find mapped input %s", name)
	}

	// mapped path already resolved to a absolute path
	p := resolvedInput.ResolvedPath
	relPath := resolvedInput.UserPath
	if found {
		if resolvedInput.Type != RPackInputTypeDirectory {
			return nil, fmt.Errorf("map path %q is not a directory", name)
		}
		cleanSuffix := filepath.Clean(suffix)
		if filepath.IsAbs(cleanSuffix) {
			return nil, fmt.Errorf("map path %q needs to be relative", name)
		}
		if !filepath.IsLocal(cleanSuffix) {
			return nil, fmt.Errorf("map path %q needs to be local", name)
		}
		p = filepath.Join(p, cleanSuffix)
		relPath = filepath.Join(relPath, cleanSuffix)
	}
	return &ControlledFile{
		MapName:  resolvedInput.Name,
		AbsPath:  p,
		Path:     relPath,
		Location: FileResolverLocationMapped,
	}, nil
}

func (r *FileResolver) resolveRPackPath(name string) (*ControlledFile, error) {
	cleanPath := filepath.Clean(name)
	if filepath.IsAbs(cleanPath) {
		return nil, fmt.Errorf("rPack path %q needs to be relative", name)
	}
	if !filepath.IsLocal(cleanPath) {
		return nil, fmt.Errorf("rPack path %q needs to be local", name)
	}

	return &ControlledFile{
		AbsPath:  filepath.Join(r.defSourcePath, cleanPath),
		Path:     cleanPath,
		Location: FileResolverLocationRPack,
	}, nil
}

func (r *FileResolver) resolveTempPath(name string) (*ControlledFile, error) {
	cleanPath := filepath.Clean(name)
	if filepath.IsAbs(cleanPath) {
		return nil, fmt.Errorf("temp path %q needs to be relative", name)
	}
	if !filepath.IsLocal(cleanPath) {
		return nil, fmt.Errorf("temp input %q needs to be local", name)
	}
	return &ControlledFile{
		AbsPath:  filepath.Join(r.tempPath, cleanPath),
		Path:     cleanPath,
		Location: FileResolverLocationTemp,
	}, nil
}

// ResolveOutput resolves user defined file paths from script to absolute paths mapping to different locations.
func (r *FileResolver) ResolveOutput(name string) (*ControlledFile, error) {
	prefix, suffix, found := strings.Cut(name, ":")
	if found {
		if prefix == "temp" {
			// Resolve file to the temp directory
			return r.resolveTempPath(suffix)
		}
		return nil, fmt.Errorf("output path needs to use temp: prefix or no prefix at all")
	}

	cleanPath := filepath.Clean(prefix)
	if filepath.IsAbs(cleanPath) {
		return nil, fmt.Errorf("output path %q needs to be relative", name)
	}
	if !filepath.IsLocal(cleanPath) {
		return nil, fmt.Errorf("output path %q needs to be local", name)
	}
	return &ControlledFile{
		AbsPath:  filepath.Join(r.runPath, cleanPath),
		Path:     cleanPath,
		Location: FileResolverLocationSource,
	}, nil
}
