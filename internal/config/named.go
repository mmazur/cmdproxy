package config

import (
	"fmt"
	"strings"

	"github.com/mmazur/cmdproxy/internal/policy/argmatch"
)

func checkFields(table map[string]any, allowed ...string) error {
	for key := range table {
		found := false
		for _, name := range allowed {
			if key == name {
				found = true
				break
			}
		}
		if !found {
			return fmt.Errorf("unknown field %q", key)
		}
	}
	return nil
}

func stringList(value any) ([]string, error) {
	list, ok := value.([]any)
	if !ok {
		return nil, fmt.Errorf("expected an array of strings, got %T", value)
	}
	result := make([]string, len(list))
	for i, item := range list {
		s, ok := item.(string)
		if !ok {
			return nil, fmt.Errorf("[%d]: expected string, got %T", i, item)
		}
		result[i] = s
	}
	return result, nil
}

func parseNamedRule(table map[string]any, valueSets map[string][]string) (*argmatch.NamedRule, error) {
	if err := checkFields(table, "prefix", "options", "positionals", "after_double_dash"); err != nil {
		return nil, err
	}
	var spec argmatch.NamedSpec
	for _, field := range []struct {
		name string
		dest *[]string
	}{
		{"prefix", &spec.Prefix},
		{"positionals", &spec.Positionals},
		{"after_double_dash", &spec.AfterDoubleDash},
	} {
		if raw, exists := table[field.name]; exists {
			patterns, err := stringList(raw)
			if err != nil {
				return nil, fmt.Errorf("%s: %w", field.name, err)
			}
			*field.dest = patterns
		}
	}
	if raw, exists := table["options"]; exists {
		options, ok := raw.([]any)
		if !ok {
			return nil, fmt.Errorf("options: expected array, got %T", raw)
		}
		for i, item := range options {
			opt, err := parseOption(item, valueSets)
			if err != nil {
				return nil, fmt.Errorf("options[%d]: %w", i, err)
			}
			spec.Options = append(spec.Options, opt)
		}
	}
	return argmatch.CompileNamed(spec)
}

func parseOption(raw any, valueSets map[string][]string) (argmatch.OptionSpec, error) {
	var spec argmatch.OptionSpec
	switch value := raw.(type) {
	case []any:
		list, err := stringList(value)
		if err != nil {
			return spec, err
		}
		if len(list) == 0 {
			return spec, fmt.Errorf("compact option must contain a name")
		}
		// Only literal aliases are expanded, never wildcard option names.
		name := list[0]
		if strings.HasPrefix(name, "{") && strings.HasSuffix(name, "}") {
			spec.Names = strings.Split(name[1:len(name)-1], ",")
		} else {
			spec.Names = []string{name}
		}
		spec.Values = list[1:]
	case map[string]any:
		if err := checkFields(value, "names", "values", "values_ref", "occurs"); err != nil {
			return spec, err
		}
		var err error
		spec.Names, err = stringList(value["names"])
		if err != nil {
			return spec, fmt.Errorf("names: %w", err)
		}
		patterns, hasValues := value["values"]
		reference, hasRef := value["values_ref"]
		if hasValues == hasRef {
			return spec, fmt.Errorf("exactly one of values or values_ref is required")
		}
		if hasValues {
			spec.Values, err = stringList(patterns)
			if err != nil {
				return spec, fmt.Errorf("values: %w", err)
			}
		} else {
			name, ok := reference.(string)
			if !ok {
				return spec, fmt.Errorf("values_ref: expected string, got %T", reference)
			}
			var exists bool
			spec.Values, exists = valueSets[name]
			if !exists {
				return spec, fmt.Errorf("values_ref: unknown value set %q", name)
			}
			if len(spec.Values) == 0 {
				return spec, fmt.Errorf("values_ref: value set %q must not be empty; declare switches with values = []", name)
			}
		}
		if occurs, exists := value["occurs"]; exists {
			var ok bool
			spec.Occurs, ok = occurs.(string)
			if !ok || spec.Occurs == "" {
				return spec, fmt.Errorf("occurs: expected nonempty quantifier string")
			}
		}
	default:
		return spec, fmt.Errorf("expected option array or table, got %T", raw)
	}
	return spec, nil
}
