package agent

import (
	"bytes"
	_ "embed"
	"encoding/json"
	"fmt"
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
