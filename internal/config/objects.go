package config

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// TemplateObject is an intermediate representation before template resolution.
type TemplateObject struct {
	Type       string
	Attrs      map[string]string
	CustomVars map[string]string
	File       string
	Line       int
	Resolved   bool
}

func (t *TemplateObject) Name() string {
	return t.Attrs["name"]
}

func (t *TemplateObject) Register() bool {
	v, ok := t.Attrs["register"]
	if !ok {
		return true
	}
	return v != "0"
}

func (t *TemplateObject) Get(key string) (string, bool) {
	v, ok := t.Attrs[key]
	return v, ok
}

func (t *TemplateObject) Has(key string) bool {
	_, ok := t.Attrs[key]
	return ok
}

// ObjectParser reads object definition files and produces TemplateObjects.
type ObjectParser struct {
	Objects []*TemplateObject
	// byTypeName maps "type:name" to the template object for template lookups.
	byTypeName map[string]*TemplateObject
}

func NewObjectParser() *ObjectParser {
	return &ObjectParser{
		byTypeName: make(map[string]*TemplateObject),
	}
}

// ParseFile reads a single object config file, handling include_file/include_dir.
func (p *ObjectParser) ParseFile(path string) error {
	f, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("cannot open config file %s: %w", path, err)
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	lineNum := 0
	var current *TemplateObject
	inDefinition := false

	for scanner.Scan() {
		lineNum++
		line := scanner.Text()

		// Strip inline ; comments (unless escaped with \;)
		line = stripSemicolonComment(line)
		line = strings.TrimSpace(line)

		// Skip blank lines and # comments
		if line == "" || line[0] == '#' {
			continue
		}

		// Handle include directives (only outside definitions)
		if !inDefinition {
			if strings.HasPrefix(line, "include_file=") {
				inclPath := strings.TrimSpace(line[13:])
				if !filepath.IsAbs(inclPath) {
					inclPath = filepath.Join(filepath.Dir(path), inclPath)
				}
				if err := p.ParseFile(inclPath); err != nil {
					return err
				}
				continue
			}
			if strings.HasPrefix(line, "include_dir=") {
				inclDir := strings.TrimSpace(line[12:])
				if !filepath.IsAbs(inclDir) {
					inclDir = filepath.Join(filepath.Dir(path), inclDir)
				}
				if err := p.ParseDir(inclDir); err != nil {
					return err
				}
				continue
			}
		}

		if !inDefinition {
			if strings.HasPrefix(line, "define ") {
				rest := strings.TrimSpace(line[7:])
				// Remove trailing {
				rest = strings.TrimSuffix(rest, "{")
				rest = strings.TrimSpace(rest)
				if rest == "" {
					return fmt.Errorf("%s:%d: missing object type", path, lineNum)
				}
				// Handle "hostgroupescalation" as no-op
				if rest == "hostgroupescalation" {
					inDefinition = true
					current = nil
					continue
				}
				current = &TemplateObject{
					Type:       rest,
					Attrs:      make(map[string]string),
					CustomVars: make(map[string]string),
					File:       path,
					Line:       lineNum,
				}
				inDefinition = true
			}
		} else {
			if line == "}" {
				if current != nil {
					p.Objects = append(p.Objects, current)
					if name := current.Name(); name != "" {
						key := current.Type + ":" + name
						if _, exists := p.byTypeName[key]; exists {
							return fmt.Errorf("%s:%d: duplicate template name '%s' for type '%s'", path, current.Line, name, current.Type)
						}
						p.byTypeName[key] = current
					}
				}
				current = nil
				inDefinition = false
				continue
			}
			if strings.HasPrefix(line, "define ") {
				return fmt.Errorf("%s:%d: nested object definitions not allowed", path, lineNum)
			}
			if current == nil {
				// Inside hostgroupescalation no-op
				continue
			}
			// Parse "variable<whitespace>value"
			key, val := splitDirective(line)
			if key == "" {
				continue
			}
			// Custom variables start with _
			if strings.HasPrefix(key, "_") {
				varName := strings.ToUpper(key[1:])
				current.CustomVars[varName] = val
			} else {
				// Normalize aliases
				key = normalizeAlias(current.Type, key)
				current.Attrs[key] = val
			}
		}
	}
	if inDefinition {
		return fmt.Errorf("%s: unexpected EOF inside object definition", path)
	}
	return scanner.Err()
}

// ParseDir recursively processes a directory of .cfg files.
func (p *ObjectParser) ParseDir(dir string) error {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return fmt.Errorf("cannot read config dir %s: %w", dir, err)
	}
	for _, entry := range entries {
		name := entry.Name()
		if strings.HasPrefix(name, ".") {
			continue
		}
		full := filepath.Join(dir, name)
		if entry.IsDir() {
			if err := p.ParseDir(full); err != nil {
				return err
			}
		} else if strings.HasSuffix(name, ".cfg") {
			if err := p.ParseFile(full); err != nil {
				return err
			}
		}
	}
	return nil
}

// GetTemplate finds a template object by type and name.
func (p *ObjectParser) GetTemplate(objType, name string) *TemplateObject {
	return p.byTypeName[objType+":"+name]
}

func stripSemicolonComment(line string) string {
	for i := 0; i < len(line); i++ {
		if line[i] == '\\' {
			i++ // skip escaped char
			continue
		}
		if line[i] == ';' {
			return line[:i]
		}
	}
	return line
}

func splitDirective(line string) (string, string) {
	// Split on first whitespace
	idx := strings.IndexAny(line, " \t")
	if idx < 0 {
		return line, ""
	}
	return line[:idx], strings.TrimSpace(line[idx+1:])
}

// normalizeAlias maps attribute aliases to their canonical form.
func normalizeAlias(objType, key string) string {
	switch objType {
	case "host":
		switch key {
		case "obsess":
			return "obsess_over_host"
		case "importance":
			return "hourly_value"
		}
	case "service":
		switch key {
		case "obsess":
			return "obsess_over_service"
		case "importance":
			return "hourly_value"
		case "description":
			return "service_description"
		}
	case "contact":
		switch key {
		case "contact_groups":
			return "contactgroups"
		case "minimum_value":
			return "minimum_importance"
		}
	case "hostdependency":
		switch key {
		case "host", "master_host", "master_host_name":
			return "host_name"
		case "dependent_host":
			return "dependent_host_name"
		case "hostgroup", "hostgroups":
			return "hostgroup_name"
		case "dependent_hostgroup", "dependent_hostgroups":
			return "dependent_hostgroup_name"
		case "execution_failure_criteria":
			return "execution_failure_options"
		case "notification_failure_criteria":
			return "notification_failure_options"
		}
	case "servicedependency":
		switch key {
		case "host", "master_host", "master_host_name":
			return "host_name"
		case "description", "master_description", "master_service_description":
			return "service_description"
		case "hostgroup", "hostgroups":
			return "hostgroup_name"
		case "servicegroup", "servicegroups":
			return "servicegroup_name"
		case "dependent_host":
			return "dependent_host_name"
		case "dependent_description":
			return "dependent_service_description"
		case "dependent_hostgroup", "dependent_hostgroups":
			return "dependent_hostgroup_name"
		case "dependent_servicegroup", "dependent_servicegroups":
			return "dependent_servicegroup_name"
		case "execution_failure_criteria":
			return "execution_failure_options"
		case "notification_failure_criteria":
			return "notification_failure_options"
		}
	case "hostescalation":
		switch key {
		case "host":
			return "host_name"
		case "hostgroup", "hostgroups":
			return "hostgroup_name"
		}
	case "serviceescalation":
		switch key {
		case "host":
			return "host_name"
		case "description":
			return "service_description"
		case "hostgroup", "hostgroups":
			return "hostgroup_name"
		case "servicegroup", "servicegroups":
			return "servicegroup_name"
		}
	}
	return key
}
