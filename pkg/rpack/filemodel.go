package rpack

import (
	"os"
	"path/filepath"
	"strings"

	"log/slog"

	"github.com/oleiade/lane/v2"
	"github.com/pkg/errors"
)

const (
	RPackResolver string = "rpack"
	TempResolver  string = "temp"
	MapResolver   string = "map"
	// TargetResolver maps to the rpack target
	TargetResolver string = "target"
)

type RPackFS struct {
	*BaseFS
	PureCheck *EnsurePure
	recorder  *FSRecorder
}

// Check if RPackFS satisfies FS interface
var _ = FS(&RPackFS{})

var TargetTransferHandleFilterFn = HandleFilterFn(func(typ FSAccessType, h FSHandle) bool {
	if typ != FSAccessTypeWrite {
		return false
	}
	if h.Resolver() != TargetResolver {
		return false
	}
	return true
})

// TODO: execPath not used
func NewRPackFS(enforcePure bool, defSourcePath, runPath, tempPath, execPath string, resolvedInputs []*RPackResolvedInput) *RPackFS {
	resolvers := []FSResolver{
		NewFileBackedFSResolver(RPackResolver, "rpack:", defSourcePath),
		NewFileBackedFSResolver(TempResolver, "temp:", tempPath),
		NewMapFSResolver(MapResolver, MapFSResolverPrefix, resolvedInputs),
		NewFileBackedFSResolver(TargetResolver, "", runPath),
	}

	var pureCheck *EnsurePure
	if enforcePure {
		pureCheck = &EnsurePure{}
	}

	recorder := NewFSRecorder(nil)
	hooks := []FSAccessHook{
		&RPackAccessControlFSHook{},
		pureCheck,
		recorder,
	}

	return &RPackFS{
		BaseFS: &BaseFS{
			Resolvers: resolvers,
			Hooks:     hooks,
		},
		PureCheck: pureCheck,
		recorder:  recorder,
	}
}

func (fs *RPackFS) Check() error {
	if fs.PureCheck != nil {
		return errors.Wrap(fs.PureCheck.CheckConflicts(), "Pure Fileaccess check failed")
	}
	return nil
}

func (fs *RPackFS) Recorder() *FSRecorder {
	return fs.recorder
}

// TargetWriteHandles return all FSHandles that were written
// in the process to the target.
func (fs *RPackFS) TargetWriteHandles() []FSHandle {
	var handles []FSHandle
	for _, record := range fs.recorder.Records() {
		if TargetTransferHandleFilterFn(record.Typ, record.Handle) {
			handles = append(handles, record.Handle)
		}
	}
	return handles
}

// FS represents a filesystem and all operations on individual files
// are abstracted through this FS object.
// TODO: Probably needs something like os.Open or os.OpenFile that returns a io.Reader or Writer to implement file copy efficiently
type FS interface {
	Write(name string, b []byte) error
	Read(name string) ([]byte, error)
	Stat(name string) (exists bool, dir bool, err error)
	ReadDir(name string) (_files []string, _dirs []string, _err error)
	ReadDirAll(name string) (_files []string, _dirs []string, _err error)
}

// InMemoryFS is used for debugging purposes only.
// TODO: Probably should create directories recursively on write.
type InMemoryFS struct {
	Tree map[string]*InMemoryFSEntry
}

func NewInMemoryFS() *InMemoryFS {
	return &InMemoryFS{
		Tree: make(map[string]*InMemoryFSEntry),
	}
}

type InMemoryFSEntry struct {
	Content []byte
	IsDir   bool
}

func (fs *InMemoryFS) Mkdir(name string) {
	fs.Tree[name] = &InMemoryFSEntry{
		IsDir: true,
	}
}

func (fs *InMemoryFS) Write(name string, b []byte) error {
	if _, ok := fs.Tree[name]; !ok {
		fs.Tree[name] = &InMemoryFSEntry{}
	}
	entry := fs.Tree[name]
	if entry.IsDir {
		return errors.Errorf("%s is directory", name)
	}
	entry.Content = make([]byte, len(b))
	copy(entry.Content, b)
	return nil
}
func (fs *InMemoryFS) Read(name string) ([]byte, error) {
	if _, ok := fs.Tree[name]; !ok {
		return nil, errors.Wrapf(os.ErrNotExist, "File %s does not exist", name)
	}
	entry := fs.Tree[name]
	if entry.IsDir {
		return nil, errors.Errorf("%s is directory", name)
	}
	b := make([]byte, len(entry.Content))
	copy(b, entry.Content)
	return b, nil
}

func (fs *InMemoryFS) Stat(name string) (exists bool, dir bool, err error) {
	if _, ok := fs.Tree[name]; !ok {
		return false, false, nil
	}
	entry := fs.Tree[name]
	return true, entry.IsDir, nil
}

func (fs *InMemoryFS) ReadDir(name string) (_files []string, _dirs []string, _err error) {
	return nil, nil, errors.Errorf("Not yet implemented")
}
func (fs *InMemoryFS) ReadDirAll(name string) (_files []string, _dirs []string, _err error) {
	return nil, nil, errors.Errorf("Not yet implemented")
}

// Base RPack Filesystem model.
// Resolvers resolve friendly filenames such as prefix:path to a specific location on the actual filesystem.
// Exactly one resolver is allowed to return `matched=true` for a given prefix, the first resolver matching is used to acquire a FSHandle.
// Hooks are called on any interactions with the handles and are used for recording written files
// as well as preventing unallowed access to files.
// The BaseFS does not expose FSHandles directly but the BaseFS is used for any interaction with those Handles.
type BaseFS struct {
	Resolvers []FSResolver

	// Hooks are traversed in order
	Hooks []FSAccessHook
}

// Check if BaseFS satisfies FS interface
var _ = FS(&BaseFS{})

func (fs *BaseFS) resolve(name string) (FSHandle, error) {
	for _, r := range fs.Resolvers {
		handle, resolved, err := r.Resolve(name)
		if resolved {
			return handle, err
		}
	}
	return nil, errors.Errorf("Could not resolve filename %q", name)
}

func (fs *BaseFS) Write(name string, b []byte) error {
	handle, err := fs.resolve(name)
	if err != nil {
		return err
	}
	for _, hook := range fs.Hooks {
		if err := hook.Write(handle); err != nil {
			return err
		}
	}
	return handle.Write(b)
}

func (fs *BaseFS) Read(name string) ([]byte, error) {
	handle, err := fs.resolve(name)
	if err != nil {
		return nil, err
	}
	for _, hook := range fs.Hooks {
		if err := hook.Read(handle); err != nil {
			return nil, err
		}
	}
	return handle.Read()
}

func (fs *BaseFS) Stat(name string) (exists bool, dir bool, err error) {
	handle, err := fs.resolve(name)
	if err != nil {
		return false, false, err
	}
	for _, hook := range fs.Hooks {
		if err := hook.Stat(handle); err != nil {
			return false, false, err
		}
	}
	return handle.Stat()
}

// Copy needs to be implemented on user side with read and write calls

// ReadDir reads a directory and returns the files and directories inside this directory or an error.
// The returned list of dirs does not contain the directory itself.
func (fs *BaseFS) ReadDir(name string) (_files []string, _dirs []string, _err error) {
	handle, err := fs.resolve(name)
	if err != nil {
		return nil, nil, err
	}
	for _, hook := range fs.Hooks {
		if err := hook.Stat(handle); err != nil {
			return nil, nil, err
		}
	}
	exists, dir, err := handle.Stat()
	if err != nil {
		return nil, nil, err
	}
	if !exists {
		return nil, nil, errors.Errorf("Path does not exist: %s", name)
	}
	if !dir {
		return nil, nil, errors.Errorf("Path is not a directory: %s", name)
	}

	// Call ReadDir
	for _, hook := range fs.Hooks {
		if err := hook.ReadDir(handle); err != nil {
			return nil, nil, err
		}
	}
	files, dirs, err := handle.ReadDir()
	if err != nil {
		return nil, nil, err
	}
	var namesFile []string
	var namesDir []string
	for _, handle := range files {
		for _, hook := range fs.Hooks {
			if err := hook.Stat(handle); err != nil {
				return nil, nil, err
			}
		}
		// Implicitely already called stat due to ReadDir, not doing it extra
		namesFile = append(namesFile, handle.FriendlyPath())
	}
	for _, handle := range dirs {
		for _, hook := range fs.Hooks {
			if err := hook.Stat(handle); err != nil {
				return nil, nil, err
			}
		}
		// Implicitely already called stat due to ReadDir, not doing it extra
		namesDir = append(namesDir, handle.FriendlyPath())
	}
	return namesFile, namesDir, nil
}

// ReadDirAll recursively lists all files and directories
func (fs *BaseFS) ReadDirAll(name string) (_files []string, _dirs []string, _err error) {
	var files []string
	var dirs []string

	queue := lane.NewQueue[string]()
	queue.Enqueue(name)

	for {
		cur, ok := queue.Dequeue()
		if !ok {
			break
		}

		newFiles, newDirs, err := fs.ReadDir(cur)
		if err != nil {
			return nil, nil, err
		}
		files = append(files, newFiles...)
		dirs = append(dirs, newDirs...)
		for _, dir := range newDirs {
			queue.Enqueue(dir)
		}
	}

	return files, dirs, nil
}

// Options to implement:
// Accesscontrol part of FS by executing HandleFuncs, or additionally on every call
// Can be used to do recording as well as access control
type FSAccessHook interface {
	Read(FSHandle) error
	Write(FSHandle) error
	ReadDir(FSHandle) error
	Stat(FSHandle) error
}

// FSResolver resolves a friendly name such as prefix:path to a FSHandle.
// If signals using the `matched` result if the resolver should match the name
// or if another resolver should be used.
type FSResolver interface {
	Resolve(name string) (h FSHandle, matched bool, err error)
}

// FileBackedFSResolver handles paths in the form of prefix:path mappend to baseDir/path
// using simple filepath actions.
// Implements FSResolver.
type FileBackedFSResolver struct {
	name    string
	prefix  string
	baseDir string
}

// Check FileBackedFSResolver satisfies FSResolver interface
var _ = FSResolver(&FileBackedFSResolver{})

func NewFileBackedFSResolver(name string, prefix string, baseDir string) *FileBackedFSResolver {
	return &FileBackedFSResolver{
		name:    name,
		prefix:  prefix,
		baseDir: baseDir,
	}
}

func (r *FileBackedFSResolver) Resolve(name string) (FSHandle, bool, error) {
	suffix, found := strings.CutPrefix(name, r.prefix)
	if !found {
		return nil, false, nil // Do not match
	}

	cleanPath := filepath.Clean(suffix)
	if filepath.IsAbs(cleanPath) {
		return nil, true, errors.Errorf("Path %q needs to be relative", name)
	}
	if !filepath.IsLocal(cleanPath) {
		return nil, true, errors.Errorf("Path %q needs to be local", name)
	}
	absPath := filepath.Join(r.baseDir, cleanPath)
	friendlyPath := r.prefix + cleanPath
	indirectTargetPath := cleanPath
	return NewFileBackedFSHandle(absPath, friendlyPath, r.name, indirectTargetPath), true, nil
}

const MapFSResolverPrefix = "map:"

type MapFSResolver struct {
	name           string
	prefix         string
	resolvedInputs []*RPackResolvedInput
}

// Check MapFSResolver satisfies FSResolver interface
var _ = FSResolver(&MapFSResolver{})

func NewMapFSResolver(name string, prefix string, resolvedInputs []*RPackResolvedInput) *MapFSResolver {
	return &MapFSResolver{
		name:           name,
		prefix:         prefix,
		resolvedInputs: resolvedInputs,
	}
}

func (r *MapFSResolver) Resolve(name string) (FSHandle, bool, error) {
	suffix, found := strings.CutPrefix(name, r.prefix)
	if !found {
		return nil, false, nil // Do not match
	}

	cleanPath := filepath.Clean(suffix)
	if filepath.IsAbs(cleanPath) {
		return nil, true, errors.Errorf("Path %q needs to be relative", name)
	}
	if !filepath.IsLocal(cleanPath) {
		return nil, true, errors.Errorf("Path %q needs to be local", name)
	}

	base, nextPath, found := strings.Cut(suffix, "/")
	// Resolve prefix first, it is always given
	var resolvedInput *RPackResolvedInput
	for _, ri := range r.resolvedInputs {
		if ri.Name == base {
			resolvedInput = ri
			break
		}
	}
	if resolvedInput == nil {
		return nil, true, errors.Errorf("Could not find mapped input %s", name)
	}

	// mapped path already resolved to a absolute path
	p := resolvedInput.ResolvedPath
	relPath := resolvedInput.UserPath
	// TODO: CleanPath is already full path, maybe we want to build it by hand and only create short clean Name first
	cleanFriendlyName := r.prefix + cleanPath
	if found {
		if resolvedInput.Type != RPackInputTypeDirectory {
			return nil, true, errors.Errorf("Map path %q is not a directory", name)
		}
		cleanNextPath := filepath.Clean(nextPath)
		if filepath.IsAbs(cleanNextPath) {
			return nil, true, errors.Errorf("Map path %q needs to be relative", name)
		}
		if !filepath.IsLocal(cleanNextPath) {
			return nil, true, errors.Errorf("Map path %q needs to be local", name)
		}
		p = filepath.Join(p, cleanNextPath)
		relPath = filepath.Join(relPath, cleanNextPath)
	}

	slog.Debug("MapFSResolver: Create new fshandle", "friendlyname", cleanFriendlyName, "resolver", r.name, "relPath", relPath, "absPath", p)
	return NewFileBackedFSHandle(p, cleanFriendlyName, r.name, relPath), true, nil
}

type FSAccessType string

const (
	FSAccessTypeRead    FSAccessType = "read"
	FSAccessTypeWrite   FSAccessType = "write"
	FSAccessTypeStat    FSAccessType = "stat"
	FSAccessTypeReadDir FSAccessType = "readdir"
)

func (t FSAccessType) String() string {
	return string(t)
}

// HandleFilterFn is used to filter FSHandles
type HandleFilterFn func(FSAccessType, FSHandle) bool

// FSRecorder records all filesystem access
// passing a filter function and makes the results
// available through Records().
type FSRecorder struct {
	filterFn HandleFilterFn
	records  []FSRecorderRecord
}

// Check FSRecorder satisfies FSAccessHook interface
var _ = FSAccessHook(&FSRecorder{})

// NewFSRecorder creates a new FSRecorder capturing all file interactions.
// If filterFn is nil, all interactions are recorded.
func NewFSRecorder(filterFn HandleFilterFn) *FSRecorder {
	return &FSRecorder{
		filterFn: filterFn,
	}
}

type FSRecorderRecord struct {
	Typ    FSAccessType
	Handle FSHandle
}

func (f *FSRecorder) Records() []FSRecorderRecord {
	return f.records
}

func (f *FSRecorder) filterRecord(typ FSAccessType, h FSHandle) {
	if f.filterFn == nil || f.filterFn(typ, h) {
		f.records = append(f.records, FSRecorderRecord{typ, h})
	}
}

func (f *FSRecorder) Read(h FSHandle) error {
	f.filterRecord(FSAccessTypeRead, h)
	return nil
}
func (f *FSRecorder) Write(h FSHandle) error {
	f.filterRecord(FSAccessTypeWrite, h)
	return nil
}
func (f *FSRecorder) ReadDir(h FSHandle) error {
	f.filterRecord(FSAccessTypeReadDir, h)
	return nil
}
func (f *FSRecorder) Stat(h FSHandle) error {
	f.filterRecord(FSAccessTypeStat, h)
	return nil
}

////

// RPackAccessControlFSHook controls the access to specific file locations.
// It performs the following rules:
// - Prevents writes to rpackdef and map
// - Prevents reads to target
type RPackAccessControlFSHook struct{}

// Check EnsurePure satisfies FSAccessHook interface
var _ = FSAccessHook(&RPackAccessControlFSHook{})

func (f *RPackAccessControlFSHook) Read(h FSHandle) error {
	resolver := h.Resolver()
	if resolver == TargetResolver {
		return errors.Errorf("Not allowed to read %s (no access to read from target directory, use 'rpack:' instead)", h.FriendlyPath())
	}
	return nil
}
func (f *RPackAccessControlFSHook) Write(h FSHandle) error {
	resolver := h.Resolver()
	switch resolver {
	case RPackResolver:
		return errors.Errorf("Not allowed to write %s, use `temp` instead", h.FriendlyPath())
	case MapResolver:
		return errors.Errorf("Not allowed to write %s, use `target` instead", h.FriendlyPath())

	}
	return nil
}
func (f *RPackAccessControlFSHook) ReadDir(h FSHandle) error {
	resolver := h.Resolver()
	if resolver == TargetResolver {
		return errors.Errorf("Not allowed to readdir %s (no access to read from target directory, use 'rpack:' instead)", h.FriendlyPath())
	}
	return nil
}
func (f *RPackAccessControlFSHook) Stat(h FSHandle) error {
	resolver := h.Resolver()
	if resolver == TargetResolver {
		return errors.Errorf("Not allowed to stat %s (no access to read from target directory, use 'rpack:' instead)", h.FriendlyPath())
	}
	return nil
}

// EnforcePure ensures operations are pure, meaning side-effect free.
// This specifically means it is not allowed to write to a file that was read before.
// Since this would lead to a second execution not being idempotent.
// Example:
// - Same file: The user reads map:mylist.yaml and writes ./mylist.yaml, mapping to the same file and a second run results in a different outcome.
// - Dir access: The user readdir map:mydir and then writes ./mydir/mylist.yaml.
// - Bootstrap files: The user stats map:mylist.yaml, if it does not exist it writes ./mylist.yaml
// It is not important in which order the read and write happens, since the first run could execute the write, while the second does the read.
// Example wrong order:
// - Same file: The user writes ./mylist.yaml, afterwards it reads map:mylist.yaml. On the second run it reads what was previously written
type EnsurePure struct {
	ReadHandles    []FSHandle
	ReadDirHandles []FSHandle
	StatHandles    []FSHandle
	WriteHandles   []FSHandle
}

// CheckConflicts checks if there exists a read/write conflict that would
// affect pureness of execution. Meaning a file was written that was read before or vice versa.
func (f *EnsurePure) CheckConflicts() error {
	// Check reads against writes
	for _, rh := range f.ReadHandles {
		readPath := rh.IndirectTargetPath()
		for _, wh := range f.WriteHandles {
			writePath := wh.IndirectTargetPath()
			if readPath == writePath {
				return errors.Errorf("Read of %s and write of same file %s not allowed", rh.FriendlyPath(), wh.FriendlyPath())
			}
		}
	}

	// Check stats against writes
	for _, sh := range f.StatHandles {
		statPath := sh.IndirectTargetPath()
		for _, wh := range f.WriteHandles {
			writePath := wh.IndirectTargetPath()
			if statPath == writePath {
				return errors.Errorf("Stat on %s and write on same file %s not allowed", sh.FriendlyPath(), wh.FriendlyPath())
			}
		}
	}

	// Check readdir against writes
	for _, rdh := range f.ReadDirHandles {
		readDirPath := rdh.IndirectTargetPath()
		for _, wh := range f.WriteHandles {
			writePath := wh.IndirectTargetPath()
			if match, err := filepath.Match(filepath.Join(readDirPath, "*"), writePath); err != nil {
				return errors.Wrapf(err, "ReadDir on %s error for pure-check against %s", rdh.FriendlyPath(), wh.FriendlyPath())
			} else if match {
				return errors.Errorf("ReadDir on %s and write on same directory %s not allowed", rdh.FriendlyPath(), wh.FriendlyPath())
			}
		}
	}

	return nil
}

// Check EnsurePure satisfies FSAccessHook interface
var _ = FSAccessHook(&EnsurePure{})

func (f *EnsurePure) Read(h FSHandle) error {
	resolver := h.Resolver()
	if resolver == MapResolver {
		f.ReadHandles = append(f.ReadHandles, h)
	}
	return nil
}
func (f *EnsurePure) Write(h FSHandle) error {
	resolver := h.Resolver()
	if resolver == TargetResolver {
		f.WriteHandles = append(f.WriteHandles, h)
	}
	return nil
}
func (f *EnsurePure) ReadDir(h FSHandle) error {
	resolver := h.Resolver()
	if resolver == MapResolver {
		f.ReadDirHandles = append(f.ReadDirHandles, h)
	}
	return nil
}
func (f *EnsurePure) Stat(h FSHandle) error {
	resolver := h.Resolver()
	if resolver == MapResolver {
		f.StatHandles = append(f.StatHandles, h)
	}
	return nil
}
