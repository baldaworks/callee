package agent

import (
	"bytes"
	_ "embed"
	"encoding/json"
	"fmt"
	"strings"
	"sync"

	"github.com/santhosh-tekuri/jsonschema/v6"
)

const schemaURL = "https://callee.metalagman.dev/schema/v1alpha1/agent.json"

//go:embed schema.json
var schemaBytes []byte

var (
	compiledSchema     *jsonschema.Schema
	compiledSchemaErr  error
	compiledSchemaOnce sync.Once
)

// Schema returns a copy of the exact embedded Draft 2020-12 schema bytes.
func Schema() []byte {
	return append([]byte(nil), schemaBytes...)
}

// SchemaForKind returns a standalone Draft 2020-12 schema document for one
// supported Callee kind, derived from the embedded full schema.
func SchemaForKind(kind Kind) ([]byte, error) {
	defName, err := schemaDefinitionName(kind)
	if err != nil {
		return nil, err
	}

	var full map[string]any
	if err := json.Unmarshal(schemaBytes, &full); err != nil {
		return nil, fmt.Errorf("decode embedded agent schema: %w", err)
	}

	definitions, ok := full["$defs"].(map[string]any)
	if !ok {
		return nil, fmt.Errorf("embedded agent schema is missing $defs")
	}

	selected := make(map[string]any)
	if err := collectSchemaDefinition(definitions, defName, selected); err != nil {
		return nil, err
	}

	document := map[string]any{
		"$schema": full["$schema"],
		"$ref":    "#/$defs/" + defName,
		"$defs":   selected,
	}

	formatted, err := json.MarshalIndent(document, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("marshal %s schema: %w", kind, err)
	}

	return append(formatted, '\n'), nil
}

func validateSchema(resource Resource) error {
	return validateSchemaDocument(resource)
}

func validateSchemaDocument(document any) error {
	schema, err := loadSchema()
	if err != nil {
		return err
	}

	data, err := json.Marshal(document)
	if err != nil {
		return fmt.Errorf("marshal canonical resource: %w", err)
	}

	value, err := jsonschema.UnmarshalJSON(bytes.NewReader(data))
	if err != nil {
		return fmt.Errorf("decode canonical resource: %w", err)
	}

	return schema.Validate(value)
}

func loadSchema() (*jsonschema.Schema, error) {
	compiledSchemaOnce.Do(func() {
		value, err := jsonschema.UnmarshalJSON(bytes.NewReader(schemaBytes))
		if err != nil {
			compiledSchemaErr = fmt.Errorf("decode embedded agent schema: %w", err)

			return
		}

		compiler := jsonschema.NewCompiler()
		compiler.DefaultDraft(jsonschema.Draft2020)

		if err := compiler.AddResource(schemaURL, value); err != nil {
			compiledSchemaErr = fmt.Errorf("register embedded agent schema: %w", err)

			return
		}

		compiledSchema, compiledSchemaErr = compiler.Compile(schemaURL)
		if compiledSchemaErr != nil {
			compiledSchemaErr = fmt.Errorf("compile embedded agent schema: %w", compiledSchemaErr)
		}
	})

	return compiledSchema, compiledSchemaErr
}

func schemaDefinitionName(kind Kind) (string, error) {
	switch kind {
	case RoleKind:
		return "role", nil
	case SequentialKind:
		return "sequential", nil
	case LoopKind:
		return "loop", nil
	default:
		return "", fmt.Errorf("unsupported kind %q (want Role, Sequential, or Loop)", kind)
	}
}

func collectSchemaDefinition(definitions map[string]any, name string, selected map[string]any) error {
	if _, exists := selected[name]; exists {
		return nil
	}

	definition, ok := definitions[name]
	if !ok {
		return fmt.Errorf("embedded agent schema is missing $defs.%s", name)
	}

	selected[name] = definition

	return walkSchemaReferences(definition, func(reference string) error {
		if reference == name {
			return nil
		}

		return collectSchemaDefinition(definitions, reference, selected)
	})
}

func walkSchemaReferences(value any, visit func(string) error) error {
	switch typed := value.(type) {
	case map[string]any:
		if ref, ok := typed["$ref"].(string); ok {
			name, local := localDefinitionName(ref)
			if local {
				if err := visit(name); err != nil {
					return err
				}
			}
		}

		for _, child := range typed {
			if err := walkSchemaReferences(child, visit); err != nil {
				return err
			}
		}
	case []any:
		for _, child := range typed {
			if err := walkSchemaReferences(child, visit); err != nil {
				return err
			}
		}
	}

	return nil
}

func localDefinitionName(reference string) (string, bool) {
	const prefix = "#/$defs/"

	if !strings.HasPrefix(reference, prefix) {
		return "", false
	}

	return strings.TrimPrefix(reference, prefix), true
}
