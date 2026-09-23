package agent

import "fmt"

// RenderDynamicProvider returns a copy of a DynamicRole with every provider
// field rendered and validated for one visit.
func (r Resource) RenderDynamicProvider(data TemplateData) (Resource, error) {
	if r.Kind != DynamicRoleKind {
		return Resource{}, fmt.Errorf("agent %q: DynamicRole is required to render a dynamic provider", r.ID)
	}

	if r.Spec.Provider == nil {
		return Resource{}, fmt.Errorf("agent %q: DynamicRole requires spec.provider", r.ID)
	}

	provider := *r.Spec.Provider
	provider.ExtraArgs = append([]string(nil), provider.ExtraArgs...)

	fields := []struct {
		path   string
		source string
		set    func(string)
	}{
		{path: "type", source: provider.Type, set: func(value string) { provider.Type = value }},
		{path: "cmd", source: provider.Cmd, set: func(value string) { provider.Cmd = value }},
		{path: "model", source: provider.Model, set: func(value string) { provider.Model = value }},
		{path: "reasoning", source: provider.Reasoning, set: func(value string) { provider.Reasoning = value }},
		{path: "mode", source: provider.Mode, set: func(value string) { provider.Mode = value }},
	}

	for _, field := range fields {
		value, err := renderDynamicProviderField(r.ID+" spec.provider."+field.path, field.source, data)
		if err != nil {
			return Resource{}, err
		}

		field.set(value)
	}

	for index, source := range provider.ExtraArgs {
		value, err := renderDynamicProviderField(fmt.Sprintf("%s spec.provider.extraArgs[%d]", r.ID, index), source, data)
		if err != nil {
			return Resource{}, err
		}

		provider.ExtraArgs[index] = value
	}

	timeout, err := renderDynamicProviderField(r.ID+" spec.provider.timeout", provider.Timeout, data)
	if err != nil {
		return Resource{}, err
	}

	provider.Timeout = timeout

	effective := r
	effective.Spec.Provider = &provider

	if err := effective.validateConcreteProviderWithDiagnostics(true); err != nil {
		return Resource{}, err
	}

	return effective, nil
}

func (r Resource) validateDynamicProviderTemplates() error {
	provider := r.Spec.Provider
	if provider == nil {
		return nil
	}

	fields := []struct {
		path   string
		source string
	}{
		{path: "type", source: provider.Type},
		{path: "cmd", source: provider.Cmd},
		{path: "model", source: provider.Model},
		{path: "reasoning", source: provider.Reasoning},
		{path: "mode", source: provider.Mode},
	}

	for _, field := range fields {
		if _, err := ParseTemplate(r.ID+" spec.provider."+field.path, field.source); err != nil {
			return err
		}
	}

	for index, source := range provider.ExtraArgs {
		if _, err := ParseTemplate(fmt.Sprintf("%s spec.provider.extraArgs[%d]", r.ID, index), source); err != nil {
			return err
		}
	}

	_, err := ParseTemplate(r.ID+" spec.provider.timeout", provider.Timeout)

	return err
}

func renderDynamicProviderField(name, source string, data TemplateData) (string, error) {
	parsed, err := ParseTemplate(name, source)
	if err != nil {
		return "", err
	}

	value, err := RenderTemplate(parsed, data)
	if err != nil {
		return "", fmt.Errorf("render %s: template execution failed", name)
	}

	return value, nil
}
