package rpack

import (
	"fmt"
	"slices"
	"strings"

	lua "github.com/yuin/gopher-lua"
	"sigs.k8s.io/yaml"
)

const definitionContractV1 = "v1"

// DefinitionContract contains the version-specific public behavior selected by
// an rpack definition. Keeping the selected contract as one object prevents
// version checks from spreading through loading and execution code.
type DefinitionContract struct {
	definitionSchema   SchemaValidator
	decodeDefinition   func([]byte) (*RPackDef, error)
	newConfigValidator func([]byte, string) (SchemaValidator, error)
	openLuaLibraries   func(*lua.LState) error
	luaFunctions       func(*LuaModel) map[string]lua.LGFunction
	newFS              func(*definitionFSConfig) *RPackFS
	version            string
	luaModuleName      string
}

type definitionFSConfig struct {
	defSourcePath  string
	runPath        string
	tempPath       string
	execPath       string
	resolvedInputs []*RPackResolvedInput
	enforcePure    bool
}

var definitionContracts = newDefinitionContractRegistry(
	&DefinitionContract{
		version:            definitionContractV1,
		definitionSchema:   RPackDefSchemaValidator,
		decodeDefinition:   decodeDefinitionContractV1,
		newConfigValidator: definitionContractV1ConfigValidator,
		openLuaLibraries:   definitionContractV1LuaLibraries,
		luaModuleName:      "rpack.v1",
		luaFunctions:       definitionContractV1LuaFunctions,
		newFS:              definitionContractV1FS,
	},
)

func newDefinitionContractRegistry(contracts ...*DefinitionContract) map[string]*DefinitionContract {
	registry := make(map[string]*DefinitionContract, len(contracts))
	for _, contract := range contracts {
		validateDefinitionContract(contract)
		if _, exists := registry[contract.version]; exists {
			panic(fmt.Sprintf("duplicate definition contract %q", contract.version))
		}
		registry[contract.version] = contract
	}
	return registry
}

func validateDefinitionContract(contract *DefinitionContract) {
	if contract == nil {
		panic("nil definition contract")
	}
	if contract.version == "" {
		panic("definition contract version is empty")
	}
	if contract.definitionSchema == nil {
		panic(fmt.Sprintf("definition contract %q has no definition schema", contract.version))
	}
	if contract.decodeDefinition == nil {
		panic(fmt.Sprintf("definition contract %q has no definition decoder", contract.version))
	}
	if contract.newConfigValidator == nil {
		panic(fmt.Sprintf("definition contract %q has no config validator factory", contract.version))
	}
	if contract.openLuaLibraries == nil {
		panic(fmt.Sprintf("definition contract %q has no Lua library opener", contract.version))
	}
	if contract.luaModuleName == "" {
		panic(fmt.Sprintf("definition contract %q has no Lua module", contract.version))
	}
	if contract.luaFunctions == nil {
		panic(fmt.Sprintf("definition contract %q has no Lua functions", contract.version))
	}
	if contract.newFS == nil {
		panic(fmt.Sprintf("definition contract %q has no filesystem factory", contract.version))
	}
}

func decodeDefinitionContractV1(data []byte) (*RPackDef, error) {
	var def RPackDef
	if err := yaml.UnmarshalStrict(data, &def); err != nil {
		return nil, err
	}
	return &def, nil
}

func definitionContractV1ConfigValidator(data []byte, path string) (SchemaValidator, error) {
	return NewCueValidator(data, path)
}

func definitionContractV1LuaLibraries(state *lua.LState) error {
	return openLibs(state, RegisterFilepath("filepath"))
}

func definitionContractV1FS(config *definitionFSConfig) *RPackFS {
	return NewRPackFS(
		config.enforcePure,
		config.defSourcePath,
		config.runPath,
		config.tempPath,
		config.execPath,
		config.resolvedInputs,
	)
}

func definitionContractFor(version string) (*DefinitionContract, error) {
	if contract, ok := definitionContracts[version]; ok {
		return contract, nil
	}
	return nil, fmt.Errorf(
		"unsupported rpack definition contract %q (supported: %s)",
		version,
		strings.Join(supportedDefinitionContractVersions(), ", "),
	)
}

func supportedDefinitionContractVersions() []string {
	versions := make([]string, 0, len(definitionContracts))
	for version := range definitionContracts {
		versions = append(versions, version)
	}
	slices.Sort(versions)
	return versions
}

func (c *DefinitionContract) validateDefinition(def *RPackDef) error {
	document := any(def)
	if def.contractDocument != nil {
		document = def.contractDocument
	}
	if err := c.definitionSchema.Validate(document); err != nil {
		return fmt.Errorf("validating rpack definition failed: %w", err)
	}
	return nil
}
