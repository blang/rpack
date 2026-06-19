package rpack

import (
	"errors"
	"testing"
)

// mockFSHandle is a minimal FSHandle implementation for testing purity checks.
type mockFSHandle struct {
	resolver           string
	friendlyPath       string
	indirectTargetPath string
}

func (m *mockFSHandle) Resolver() string           { return m.resolver }
func (m *mockFSHandle) FriendlyPath() string       { return m.friendlyPath }
func (m *mockFSHandle) IndirectTargetPath() string { return m.indirectTargetPath }
func (m *mockFSHandle) Read() ([]byte, error)      { return nil, nil }
func (m *mockFSHandle) Write([]byte) error         { return nil }
func (m *mockFSHandle) Stat() (exists, dir bool, err error) {
	return false, false, nil
}
func (m *mockFSHandle) ReadDir() (files, dirs []FSHandle, err error) {
	return nil, nil, nil
}
func (m *mockFSHandle) Transfer(string) error { return nil }

// TestRPackFSCheck tests the RPackFS.Check() method.
// This is a regression test for the bug where Check() always returned an error
// when PureCheck was non-nil, even when CheckConflicts() returned nil.
func TestRPackFSCheck(t *testing.T) {
	tests := []struct {
		pureCheck   *EnsurePure
		name        string
		expectError bool
	}{
		{
			name:      "nil PureCheck returns nil",
			pureCheck: nil,
		},
		{
			name:      "PureCheck with no conflicts returns nil",
			pureCheck: &EnsurePure{},
		},
		{
			name: "PureCheck with read/write conflict returns error",
			pureCheck: &EnsurePure{
				ReadHandles: []FSHandle{
					&mockFSHandle{
						resolver:           MapResolver,
						friendlyPath:       "map:config.yaml",
						indirectTargetPath: "config.yaml",
					},
				},
				WriteHandles: []FSHandle{
					&mockFSHandle{
						resolver:           TargetResolver,
						friendlyPath:       "config.yaml",
						indirectTargetPath: "config.yaml",
					},
				},
			},
			expectError: true,
		},
		{
			name: "PureCheck with stat/write conflict returns error",
			pureCheck: &EnsurePure{
				StatHandles: []FSHandle{
					&mockFSHandle{
						resolver:           MapResolver,
						friendlyPath:       "map:config.yaml",
						indirectTargetPath: "config.yaml",
					},
				},
				WriteHandles: []FSHandle{
					&mockFSHandle{
						resolver:           TargetResolver,
						friendlyPath:       "config.yaml",
						indirectTargetPath: "config.yaml",
					},
				},
			},
			expectError: true,
		},
		{
			name: "PureCheck with readdir/write conflict returns error",
			pureCheck: &EnsurePure{
				ReadDirHandles: []FSHandle{
					&mockFSHandle{
						resolver:           MapResolver,
						friendlyPath:       "map:configs",
						indirectTargetPath: "configs",
					},
				},
				WriteHandles: []FSHandle{
					&mockFSHandle{
						resolver:           TargetResolver,
						friendlyPath:       "configs/new.yaml",
						indirectTargetPath: "configs/new.yaml",
					},
				},
			},
			expectError: true,
		},
		{
			name: "PureCheck with non-conflicting paths returns nil",
			pureCheck: &EnsurePure{
				ReadHandles: []FSHandle{
					&mockFSHandle{
						resolver:           MapResolver,
						friendlyPath:       "map:input.yaml",
						indirectTargetPath: "input.yaml",
					},
				},
				WriteHandles: []FSHandle{
					&mockFSHandle{
						resolver:           TargetResolver,
						friendlyPath:       "output.yaml",
						indirectTargetPath: "output.yaml",
					},
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fs := &RPackFS{
				PureCheck: tt.pureCheck,
			}
			err := fs.Check()

			if tt.expectError && err == nil {
				t.Errorf("expected error but got nil")
			}
			if !tt.expectError && err != nil {
				t.Errorf("expected no error but got: %v", err)
			}
			if tt.expectError && err != nil {
				if !errors.Is(err, ErrPurityCheck) {
					t.Errorf("expected error to wrap ErrPurityCheck, got: %v", err)
				}
			}
		})
	}
}

// TestEnsurePureCheckConflicts tests the EnsurePure.CheckConflicts() method directly.
func TestEnsurePureCheckConflicts(t *testing.T) {
	tests := []struct {
		pure        *EnsurePure
		name        string
		errorMsg    string
		expectError bool
	}{
		{
			name: "empty handles returns nil",
			pure: &EnsurePure{},
		},
		{
			name: "only reads returns nil",
			pure: &EnsurePure{
				ReadHandles: []FSHandle{
					&mockFSHandle{
						resolver:           MapResolver,
						friendlyPath:       "map:input.yaml",
						indirectTargetPath: "input.yaml",
					},
				},
			},
		},
		{
			name: "only writes returns nil",
			pure: &EnsurePure{
				WriteHandles: []FSHandle{
					&mockFSHandle{
						resolver:           TargetResolver,
						friendlyPath:       "output.yaml",
						indirectTargetPath: "output.yaml",
					},
				},
			},
		},
		{
			name: "read/write same path returns error",
			pure: &EnsurePure{
				ReadHandles: []FSHandle{
					&mockFSHandle{
						resolver:           MapResolver,
						friendlyPath:       "map:data.yaml",
						indirectTargetPath: "data.yaml",
					},
				},
				WriteHandles: []FSHandle{
					&mockFSHandle{
						resolver:           TargetResolver,
						friendlyPath:       "data.yaml",
						indirectTargetPath: "data.yaml",
					},
				},
			},
			expectError: true,
			errorMsg:    "read of map:data.yaml and write of same file data.yaml not allowed",
		},
		{
			name: "stat/write same path returns error",
			pure: &EnsurePure{
				StatHandles: []FSHandle{
					&mockFSHandle{
						resolver:           MapResolver,
						friendlyPath:       "map:config.yaml",
						indirectTargetPath: "config.yaml",
					},
				},
				WriteHandles: []FSHandle{
					&mockFSHandle{
						resolver:           TargetResolver,
						friendlyPath:       "config.yaml",
						indirectTargetPath: "config.yaml",
					},
				},
			},
			expectError: true,
			errorMsg:    "stat on map:config.yaml and write on same file config.yaml not allowed",
		},
		{
			name: "readdir/write in directory returns error",
			pure: &EnsurePure{
				ReadDirHandles: []FSHandle{
					&mockFSHandle{
						resolver:           MapResolver,
						friendlyPath:       "map:configs",
						indirectTargetPath: "configs",
					},
				},
				WriteHandles: []FSHandle{
					&mockFSHandle{
						resolver:           TargetResolver,
						friendlyPath:       "configs/new.yaml",
						indirectTargetPath: "configs/new.yaml",
					},
				},
			},
			expectError: true,
			errorMsg:    "readDir on map:configs and write on same directory configs/new.yaml not allowed",
		},
		{
			name: "read/write different paths returns nil",
			pure: &EnsurePure{
				ReadHandles: []FSHandle{
					&mockFSHandle{
						resolver:           MapResolver,
						friendlyPath:       "map:input.yaml",
						indirectTargetPath: "input.yaml",
					},
				},
				WriteHandles: []FSHandle{
					&mockFSHandle{
						resolver:           TargetResolver,
						friendlyPath:       "output.yaml",
						indirectTargetPath: "output.yaml",
					},
				},
			},
		},
		{
			name: "readdir/write different directories returns nil",
			pure: &EnsurePure{
				ReadDirHandles: []FSHandle{
					&mockFSHandle{
						resolver:           MapResolver,
						friendlyPath:       "map:inputs",
						indirectTargetPath: "inputs",
					},
				},
				WriteHandles: []FSHandle{
					&mockFSHandle{
						resolver:           TargetResolver,
						friendlyPath:       "outputs/result.yaml",
						indirectTargetPath: "outputs/result.yaml",
					},
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.pure.CheckConflicts()

			if tt.expectError && err == nil {
				t.Errorf("expected error but got nil")
			}
			if !tt.expectError && err != nil {
				t.Errorf("expected no error but got: %v", err)
			}
			if tt.expectError && tt.errorMsg != "" {
				if err.Error() != tt.errorMsg {
					t.Errorf("expected error message %q, got %q", tt.errorMsg, err.Error())
				}
			}
		})
	}
}
