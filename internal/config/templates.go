package config

import (
	"fmt"
	"strings"
)

// ResolveTemplates processes the 'use' directive on all objects, applying
// left-to-right template inheritance with additive (+prefix) support.
func ResolveTemplates(parser *ObjectParser) error {
	for _, obj := range parser.Objects {
		if err := resolveObject(parser, obj, nil); err != nil {
			return err
		}
	}
	// Clean additive strings (remove leading + from all values)
	for _, obj := range parser.Objects {
		cleanAdditiveStrings(obj)
	}
	return nil
}

func resolveObject(parser *ObjectParser, obj *TemplateObject, chain []*TemplateObject) error {
	if obj.Resolved {
		return nil
	}
	// Check for circular reference
	for _, c := range chain {
		if c == obj {
			return fmt.Errorf("circular template reference detected for %s '%s' at %s:%d",
				obj.Type, obj.Name(), obj.File, obj.Line)
		}
	}

	useStr, hasUse := obj.Attrs["use"]
	if !hasUse {
		obj.Resolved = true
		return nil
	}

	templates := splitCSV(useStr)
	chain = append(chain, obj)

	for _, tmplName := range templates {
		tmpl := parser.GetTemplate(obj.Type, tmplName)
		if tmpl == nil {
			return fmt.Errorf("%s:%d: template '%s' not found for type '%s'",
				obj.File, obj.Line, tmplName, obj.Type)
		}
		// Recursively resolve the template first
		if err := resolveObject(parser, tmpl, chain); err != nil {
			return err
		}
		// Inherit attributes: only if the child doesn't already have the attribute
		for key, val := range tmpl.Attrs {
			if key == "name" || key == "use" || key == "register" {
				continue
			}
			childVal, childHas := obj.Attrs[key]
			if !childHas {
				obj.Attrs[key] = val
			} else if strings.HasPrefix(childVal, "+") {
				// Additive inheritance: prepend template value
				obj.Attrs[key] = val + "," + childVal[1:]
			}
		}
		// Inherit custom vars
		for key, val := range tmpl.CustomVars {
			if _, exists := obj.CustomVars[key]; !exists {
				obj.CustomVars[key] = val
			}
		}
	}
	obj.Resolved = true
	return nil
}

func cleanAdditiveStrings(obj *TemplateObject) {
	for key, val := range obj.Attrs {
		if strings.HasPrefix(val, "+") {
			obj.Attrs[key] = val[1:]
		}
	}
}

func splitCSV(s string) []string {
	parts := strings.Split(s, ",")
	result := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			result = append(result, p)
		}
	}
	return result
}
