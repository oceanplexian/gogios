package config

import (
	"fmt"

	"github.com/oceanplexian/gogios/internal/objects"
)

// LoadResult holds the complete parsed configuration.
type LoadResult struct {
	MainCfg    *MainConfig
	UserMacros [MaxUserMacros]string
	Store      *objects.ObjectStore
}

// LoadConfig reads and processes all configuration starting from the main config file.
// This follows the Nagios startup sequence: main config -> resource files -> object files ->
// template resolution -> expansion -> registration -> validation.
func LoadConfig(mainConfigPath string) (*LoadResult, error) {
	// Step 1: Parse main config file
	mainCfg, err := ReadMainConfig(mainConfigPath)
	if err != nil {
		return nil, fmt.Errorf("error reading main config: %w", err)
	}

	// Step 2: Parse resource files
	var macros [MaxUserMacros]string
	for _, rf := range mainCfg.ResourceFiles {
		if err := ReadResourceFile(rf, &macros); err != nil {
			return nil, fmt.Errorf("error reading resource file: %w", err)
		}
	}

	// Step 3: Parse all object config files
	parser := NewObjectParser()
	for _, cf := range mainCfg.CfgFiles {
		if err := parser.ParseFile(cf); err != nil {
			return nil, fmt.Errorf("error parsing config file: %w", err)
		}
	}
	for _, cd := range mainCfg.CfgDirs {
		if err := parser.ParseDir(cd); err != nil {
			return nil, fmt.Errorf("error parsing config dir: %w", err)
		}
	}

	// Step 4: Resolve templates
	if err := ResolveTemplates(parser); err != nil {
		return nil, fmt.Errorf("error resolving templates: %w", err)
	}

	// Step 5: Expand, register, and wire up all objects
	store := objects.NewObjectStore()
	if err := ExpandAndRegister(parser, store); err != nil {
		return nil, fmt.Errorf("error expanding objects: %w", err)
	}

	return &LoadResult{
		MainCfg:    mainCfg,
		UserMacros: macros,
		Store:      store,
	}, nil
}

// VerifyConfig loads and validates configuration, returning errors found.
func VerifyConfig(mainConfigPath string) (*LoadResult, []error) {
	result, err := LoadConfig(mainConfigPath)
	if err != nil {
		return nil, []error{err}
	}
	errs := Validate(result.Store)
	return result, errs
}
