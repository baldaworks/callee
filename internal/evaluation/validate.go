package evaluation

import (
	"encoding/json"
	"fmt"
	"math"
	"reflect"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/baldaworks/callee/internal/agent"
)

const (
	roundingHalfUnit  = 0.005
	floatingTolerance = 1e-9
)

// ValidateRequest checks the completely rendered request before credential lookup.
func ValidateRequest(request Request) error {
	if !validModelIdentifier(request.Model) {
		return responseError("validate request", "model must be one nonblank identifier without whitespace or control characters")
	}

	if err := validateContent("state", request.State, false); err != nil {
		return responseError("validate request", err.Error())
	}

	if text, ok := request.State.(string); ok && strings.TrimSpace(text) == "" {
		return responseError("validate request", "state must not be blank")
	}

	if len(request.Questions) == 0 {
		return responseError("validate request", "questions must not be empty")
	}

	for id, question := range request.Questions {
		if strings.TrimSpace(id) == "" || strings.TrimSpace(id) != id {
			return responseError("validate request", fmt.Sprintf("invalid question ID %q", id))
		}

		if err := validateContent("question "+id+" instructions", question.Instructions, false); err != nil {
			return responseError("validate request", err.Error())
		}

		if text, ok := question.Instructions.(string); ok && strings.TrimSpace(text) == "" {
			return responseError("validate request", fmt.Sprintf("question %q instructions must not be blank", id))
		}

		if err := validateRenderedQuestion(id, question); err != nil {
			return responseError("validate request", err.Error())
		}
	}

	return nil
}

// ValidateResult checks a normalized result against its rendered request.
func ValidateResult(request Request, result Result) error {
	if err := validateResultIdentity(request, result); err != nil {
		return err
	}

	if len(result.Answers) != len(request.Questions) {
		return responseError("validate response", "answer IDs do not match question IDs")
	}

	for id, question := range request.Questions {
		answer, ok := result.Answers[id]
		if !ok {
			return responseError("validate response", fmt.Sprintf("missing answer %q", id))
		}

		if answer.Type != question.Type {
			return responseError("validate response", fmt.Sprintf("answer %q has type %q, want %q", id, answer.Type, question.Type))
		}

		if err := validateAnswer(id, question, answer); err != nil {
			return responseError("validate response", err.Error())
		}
	}

	for id := range result.Answers {
		if _, ok := request.Questions[id]; !ok {
			return responseError("validate response", fmt.Sprintf("unexpected answer %q", id))
		}
	}

	if result.Usage != nil {
		if result.Usage.InputTokens < 0 || result.Usage.OutputTokens < 0 {
			return responseError("validate response", "usage token counts must be nonnegative")
		}

		if result.Usage.Cost != nil && (!finite(*result.Usage.Cost) || *result.Usage.Cost < 0) {
			return responseError("validate response", "usage cost must be finite and nonnegative")
		}
	}

	return nil
}

func validateResultIdentity(request Request, result Result) error {
	if strings.TrimSpace(result.Service) == "" || strings.TrimSpace(result.RequestedModel) == "" || strings.TrimSpace(result.Model) == "" {
		return responseError("validate response", "service, requested model, and actual model must not be blank")
	}

	if result.RequestedModel != request.Model {
		return responseError("validate response", "requested model does not match the request")
	}

	if !validModelIdentifier(result.Model) {
		return responseError("validate response", "actual model must be one nonblank identifier without whitespace or control characters")
	}

	for name, value := range map[string]string{"provider": result.Provider, "request ID": result.RequestID} {
		if value != "" && (utf8.RuneCountInString(value) > 1024 || strings.IndexFunc(value, unicode.IsControl) >= 0) {
			return responseError("validate response", name+" contains unsafe metadata")
		}
	}

	return nil
}

func validModelIdentifier(model string) bool {
	return model != "" && model == strings.TrimSpace(model) &&
		strings.IndexFunc(model, func(r rune) bool { return unicode.IsSpace(r) || unicode.IsControl(r) }) < 0
}

func validateRenderedQuestion(id string, question agent.EvaluationQuestion) error {
	switch question.Type {
	case "noul":
		return validateRenderedNoul(id, question.Criteria)
	case "choice":
		return validateRenderedChoice(id, question.Criteria)
	case "score":
		return validateRenderedScore(id, question.Criteria)
	default:
		return fmt.Errorf("question %q has unsupported type %q", id, question.Type)
	}
}

func validateRenderedNoul(id string, value any) error {
	if value == nil {
		return nil
	}

	criteria, ok := value.(map[string]any)
	if !ok || len(criteria) == 0 || len(criteria) > 2 {
		return fmt.Errorf("question %q noul criteria are invalid", id)
	}

	for key, criterion := range criteria {
		if key != "true" && key != "false" {
			return fmt.Errorf("question %q has invalid noul criterion %q", id, key)
		}

		if criterion != nil {
			if err := validateContent("question "+id+" criterion "+key, criterion, false); err != nil {
				return err
			}
		}
	}

	return nil
}

func validateRenderedChoice(id string, value any) error {
	criteria, ok := value.(map[string]any)
	if !ok || len(criteria) < 2 || len(criteria) > 255 {
		return fmt.Errorf("question %q choice criteria are invalid", id)
	}

	for key, criterion := range criteria {
		if strings.TrimSpace(key) == "" || key != strings.TrimSpace(key) {
			return fmt.Errorf("question %q has invalid choice %q", id, key)
		}

		if criterion != nil {
			if err := validateContent("question "+id+" criterion "+key, criterion, false); err != nil {
				return err
			}
		}
	}

	return nil
}

func validateRenderedScore(id string, value any) error {
	criteria, ok := value.([]any)
	if !ok || len(criteria) < 2 || len(criteria) > 10 {
		return fmt.Errorf("question %q score criteria are invalid", id)
	}

	for index, criterion := range criteria {
		if criterion != nil {
			if err := validateContent(fmt.Sprintf("question %s criterion %d", id, index), criterion, false); err != nil {
				return err
			}
		}
	}

	return nil
}

func validateAnswer(id string, question agent.EvaluationQuestion, answer Answer) error {
	switch answer.Type {
	case "noul":
		return validateNoulAnswer(id, answer)
	case "choice":
		return validateChoiceAnswer(id, question.Criteria.(map[string]any), answer)
	case "score":
		return validateScoreAnswer(id, question.Criteria.([]any), answer)
	default:
		return fmt.Errorf("answer %q has unsupported type %q", id, answer.Type)
	}
}

func validateNoulAnswer(id string, answer Answer) error {
	if answer.Noul == nil || !unit(*answer.Noul) {
		return fmt.Errorf("answer %q noul must be finite and within [0,1]", id)
	}

	if answer.Choice != "" || answer.Score != nil || answer.Legend != nil || answer.Probabilities != nil || answer.Confidence != nil {
		return fmt.Errorf("answer %q noul contains fields from another answer type", id)
	}

	return nil
}

func validateChoiceAnswer(id string, criteria map[string]any, answer Answer) error {
	if answer.Noul != nil || answer.Score != nil || answer.Legend != nil || answer.Choice == "" || answer.Confidence == nil {
		return fmt.Errorf("answer %q choice has an invalid field set", id)
	}

	if _, ok := criteria[answer.Choice]; !ok {
		return fmt.Errorf("answer %q selected unknown choice %q", id, answer.Choice)
	}

	if err := validateDistribution(id, answer.Probabilities, mapKeys(criteria)); err != nil {
		return err
	}

	if !unit(*answer.Confidence) {
		return fmt.Errorf("answer %q confidence must be finite and within [0,1]", id)
	}

	selected := answer.Probabilities[answer.Choice]
	for _, probability := range answer.Probabilities {
		if probability-selected > 2*roundingHalfUnit+floatingTolerance {
			return fmt.Errorf("answer %q choice is not a maximum-probability option", id)
		}
	}

	return nil
}

func validateScoreAnswer(id string, criteria []any, answer Answer) error {
	if answer.Noul != nil || answer.Choice != "" || answer.Score == nil || answer.Confidence == nil {
		return fmt.Errorf("answer %q score has an invalid field set", id)
	}

	if !finite(*answer.Score) || *answer.Score < 0 || *answer.Score > float64(len(criteria)-1) {
		return fmt.Errorf("answer %q score is outside its rubric", id)
	}

	keys := make([]string, len(criteria))
	for index, criterion := range criteria {
		key := strconv.Itoa(index)
		keys[index] = key

		legend, ok := answer.Legend[key]
		if !ok || !reflect.DeepEqual(normalizeJSON(legend), normalizeJSON(criterion)) {
			return fmt.Errorf("answer %q legend does not match criterion %s", id, key)
		}
	}

	if len(answer.Legend) != len(criteria) {
		return fmt.Errorf("answer %q legend keys do not match its rubric", id)
	}

	if err := validateDistribution(id, answer.Probabilities, keys); err != nil {
		return err
	}

	if !unit(*answer.Confidence) {
		return fmt.Errorf("answer %q confidence must be finite and within [0,1]", id)
	}

	expected := 0.0
	for index, key := range keys {
		expected += float64(index) * answer.Probabilities[key]
	}

	tolerance := roundingHalfUnit*(1+float64(len(criteria)*(len(criteria)-1))/2) + floatingTolerance
	if math.Abs(*answer.Score-expected) > tolerance {
		return fmt.Errorf("answer %q score is inconsistent with its probabilities", id)
	}

	return nil
}

func validateDistribution(id string, probabilities map[string]float64, keys []string) error {
	if len(probabilities) != len(keys) {
		return fmt.Errorf("answer %q probability keys do not match its criteria", id)
	}

	sum := 0.0

	for _, key := range keys {
		value, ok := probabilities[key]
		if !ok || !unit(value) {
			return fmt.Errorf("answer %q has an invalid probability for %q", id, key)
		}

		sum += value
	}

	if math.Abs(sum-1) > roundingHalfUnit*float64(len(keys))+floatingTolerance {
		return fmt.Errorf("answer %q probability distribution does not sum to one", id)
	}

	return nil
}

func validateContent(path string, value any, nested bool) error {
	switch typed := value.(type) {
	case nil:
		if nested {
			return nil
		}
	case string:
		return nil
	case bool, json.Number, int, int8, int16, int32, int64, uint, uint8, uint16, uint32, uint64:
		if nested {
			return nil
		}
	case float32:
		if nested && finite(float64(typed)) {
			return nil
		}
	case float64:
		if nested && finite(typed) {
			return nil
		}
	case []any:
		for index, item := range typed {
			if err := validateContent(fmt.Sprintf("%s[%d]", path, index), item, true); err != nil {
				return err
			}
		}

		return nil
	case map[string]any:
		for key, item := range typed {
			if err := validateContent(path+"."+key, item, true); err != nil {
				return err
			}
		}

		return nil
	}

	return fmt.Errorf("%s must be finite JSON string, object, or array content", path)
}

func mapKeys(values map[string]any) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}

	return keys
}

func normalizeJSON(value any) any {
	data, err := json.Marshal(value)
	if err != nil {
		return value
	}

	var normalized any
	if json.Unmarshal(data, &normalized) != nil {
		return value
	}

	return normalized
}

func unit(value float64) bool   { return finite(value) && value >= 0 && value <= 1 }
func finite(value float64) bool { return !math.IsNaN(value) && !math.IsInf(value, 0) }

func responseError(op, message string) error {
	return &Error{Class: ErrorResponse, Op: op, Err: fmt.Errorf("%s", message)}
}
