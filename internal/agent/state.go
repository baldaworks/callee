package agent

import (
	"encoding/json"
	"fmt"
	"math"
)

func validateState(id, path string, state map[string]any) error {
	if state == nil {
		return nil
	}

	if _, ok := state["outputs"]; ok {
		return fmt.Errorf("agent %q: %s.outputs is reserved", id, path)
	}

	if err := validateStateValue(state); err != nil {
		return fmt.Errorf("agent %q: %s: %w", id, path, err)
	}

	return nil
}

func validateStateValue(value any) error {
	switch typed := value.(type) {
	case nil:
		return fmt.Errorf("null values are not supported")
	case string, bool, json.Number:
		return nil
	case int, int8, int16, int32, int64, uint, uint8, uint16, uint32, uint64:
		return nil
	case float32:
		if math.IsInf(float64(typed), 0) || math.IsNaN(float64(typed)) {
			return fmt.Errorf("numbers must be finite")
		}

		return nil
	case float64:
		if math.IsInf(typed, 0) || math.IsNaN(typed) {
			return fmt.Errorf("numbers must be finite")
		}

		return nil
	case []any:
		for index, item := range typed {
			if err := validateStateValue(item); err != nil {
				return fmt.Errorf("item %d: %w", index, err)
			}
		}

		return nil
	case map[string]any:
		for key, item := range typed {
			if err := validateStateValue(item); err != nil {
				return fmt.Errorf("field %q: %w", key, err)
			}
		}

		return nil
	default:
		return fmt.Errorf("unsupported value of type %T", value)
	}
}

func validateStateTemplates(name string, value any) error {
	switch typed := value.(type) {
	case string:
		_, err := ParseRestrictedTemplate(name, typed)

		return err
	case []any:
		for index, item := range typed {
			if err := validateStateTemplates(fmt.Sprintf("%s[%d]", name, index), item); err != nil {
				return err
			}
		}
	case map[string]any:
		for key, item := range typed {
			if err := validateStateTemplates(name+"."+key, item); err != nil {
				return err
			}
		}
	}

	return nil
}
