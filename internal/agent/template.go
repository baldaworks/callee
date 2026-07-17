package agent

import (
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"text/template"
	"text/template/parse"
	"time"

	"github.com/Masterminds/sprig/v3"
)

var parameterName = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9_-]*$`)

var legacyPromptAction = regexp.MustCompile(`{{-?\s*prompt\s*-?}}`)

const dateFunctionCount = 2

// TemplateData is the common immutable data root for every authored template.
type TemplateData struct {
	Prompt string
	Input  string
	Output string
	State  map[string]any
	Params map[string]string
}

var allowedSprigFunctions = []string{
	"abbrev", "abbrevboth", "camelcase", "cat", "contains", "hasPrefix", "hasSuffix", "indent",
	"initials", "kebabcase", "lower", "nindent", "nospace", "plural", "quote", "repeat", "replace",
	"snakecase", "squote", "substr", "swapcase", "title", "trim", "trimAll", "trimPrefix", "trimSuffix",
	"trunc", "untitle", "upper", "wrap", "wrapWith",
	"default", "empty", "coalesce", "all", "any", "ternary", "fail",
	"atoi", "int", "int64", "float64", "toDecimal", "toString", "toStrings",
	"add", "add1", "sub", "div", "mod", "mul", "addf", "add1f", "subf", "divf", "mulf", "max", "min", "maxf", "minf", "ceil", "floor", "round",
	"list", "dict", "get", "hasKey", "dig", "pick", "omit", "pluck", "append", "mustAppend",
	"prepend", "mustPrepend", "concat", "first", "mustFirst", "rest", "mustRest", "last", "mustLast",
	"initial", "mustInitial", "reverse", "mustReverse", "compact", "mustCompact", "uniq", "mustUniq",
	"without", "mustWithout", "has", "mustHas", "slice", "mustSlice", "chunk", "mustChunk", "join",
	"sortAlpha", "deepCopy", "mustDeepCopy",
	"regexMatch", "mustRegexMatch", "regexFind", "mustRegexFind", "regexFindAll", "mustRegexFindAll", "regexReplaceAll", "mustRegexReplaceAll", "regexReplaceAllLiteral", "mustRegexReplaceAllLiteral", "regexSplit", "mustRegexSplit", "regexQuoteMeta",
	"fromJson", "mustFromJson", "toJson", "mustToJson", "toPrettyJson", "mustToPrettyJson", "toRawJson", "mustToRawJson",
	"base", "dir", "clean", "ext", "isAbs",
	"semver", "semverCompare",
	"sha1sum", "sha256sum", "sha512sum", "adler32sum", "b32enc", "b32dec", "b64enc", "b64dec",
}

// FuncMap returns a fresh positive-list-only deterministic template function
// map. It never exposes Sprig's environment, clock, random, network, crypto
// generation, or mutating dictionary helpers.
func FuncMap() template.FuncMap {
	upstream := sprig.GenericFuncMap()
	functions := make(template.FuncMap, len(allowedSprigFunctions)+dateFunctionCount)

	for _, name := range allowedSprigFunctions {
		if function, ok := upstream[name]; ok {
			functions[name] = function
		}
	}

	functions["dateParse"] = dateParse
	functions["dateFormat"] = dateFormat

	return functions
}

// ParseTemplate parses one workflow-aware template with the canonical options
// and helper set. The common surface exposes Params but not Output.
func ParseTemplate(name, body string) (*template.Template, error) {
	return parseTemplate(name, body, true, false)
}

// ParseRestrictedTemplate parses a state modifier or Role parameter binding,
// where neither Params nor Output is available.
func ParseRestrictedTemplate(name, body string) (*template.Template, error) {
	return parseTemplate(name, body, false, false)
}

// ParseOutputTemplate parses a composite output template, the only surface
// where Output is available. Params remains present as an empty map.
func ParseOutputTemplate(name, body string) (*template.Template, error) {
	return parseTemplate(name, body, true, true)
}

func parseTemplate(name, body string, allowParams, allowOutput bool) (*template.Template, error) {
	parsed, err := template.New(name).Funcs(FuncMap()).Option("missingkey=zero").Parse(body)
	if err != nil {
		return nil, fmt.Errorf("parse %s: %w", name, err)
	}

	for _, defined := range parsed.Templates() {
		if defined.Tree == nil {
			continue
		}

		if !allowParams && referencesRootField(defined.Tree.Root, "Params") {
			return nil, fmt.Errorf("parse %s: .Params is unavailable on this template surface", name)
		}

		if !allowOutput && referencesRootField(defined.Tree.Root, "Output") {
			return nil, fmt.Errorf("parse %s: .Output is available only in composite spec.output", name)
		}
	}

	return parsed, nil
}

func referencesRootField(node parse.Node, name string) bool {
	if node == nil {
		return false
	}

	switch value := node.(type) {
	case *parse.ListNode:
		return listReferencesRootField(value, name)
	case *parse.ActionNode:
		return value != nil && referencesRootField(value.Pipe, name)
	case *parse.PipeNode:
		return pipeReferencesRootField(value, name)
	case *parse.CommandNode:
		return commandReferencesRootField(value, name)
	case *parse.IfNode:
		return value != nil && branchReferencesRootField(value.Pipe, value.List, value.ElseList, name)
	case *parse.RangeNode:
		return value != nil && branchReferencesRootField(value.Pipe, value.List, value.ElseList, name)
	case *parse.WithNode:
		return value != nil && branchReferencesRootField(value.Pipe, value.List, value.ElseList, name)
	case *parse.TemplateNode:
		return value != nil && referencesRootField(value.Pipe, name)
	case *parse.ChainNode:
		return value != nil && referencesRootField(value.Node, name)
	case *parse.FieldNode:
		return value != nil && len(value.Ident) > 0 && value.Ident[0] == name
	}

	return false
}

func listReferencesRootField(node *parse.ListNode, name string) bool {
	if node == nil {
		return false
	}

	for _, child := range node.Nodes {
		if referencesRootField(child, name) {
			return true
		}
	}

	return false
}

func pipeReferencesRootField(node *parse.PipeNode, name string) bool {
	if node == nil {
		return false
	}

	for _, command := range node.Cmds {
		if referencesRootField(command, name) {
			return true
		}
	}

	return false
}

func commandReferencesRootField(node *parse.CommandNode, name string) bool {
	if node == nil {
		return false
	}

	for _, argument := range node.Args {
		if referencesRootField(argument, name) {
			return true
		}
	}

	return false
}

func branchReferencesRootField(pipe *parse.PipeNode, list, elseList *parse.ListNode, name string) bool {
	return referencesRootField(pipe, name) || referencesRootField(list, name) || referencesRootField(elseList, name)
}

// RenderTemplate executes a parsed template against an immutable plain-data
// snapshot.
func RenderTemplate(parsed *template.Template, data TemplateData) (string, error) {
	snapshot, err := cloneTemplateData(data)
	if err != nil {
		return "", err
	}

	var rendered strings.Builder
	if err := parsed.Execute(&rendered, snapshot); err != nil {
		return "", fmt.Errorf("render %s: %w", parsed.Name(), err)
	}

	return rendered.String(), nil
}

// ValidateRoleTemplate enforces exactly one unconditional bare .Prompt or
// .Input primary insertion.
func ValidateRoleTemplate(id, body string) error {
	parsed, err := ParseTemplate(id+" spec.body", body)
	if err != nil {
		return err
	}

	primary, _ := primaryInsertions(parsed.Tree.Root)
	references := 0

	for _, defined := range parsed.Templates() {
		if defined.Tree != nil {
			references += primaryReferences(defined.Tree.Root)
		}
	}

	if references != 1 || primary != 1 {
		return fmt.Errorf("agent %q: Role spec.body must contain exactly one unconditional bare {{ .Prompt }} or {{ .Input }} insertion", id)
	}

	return nil
}

// ValidateRoleTemplateMigration reports legacy primary and flat parameter
// actions before the Go parser reduces them to generic undefined-function
// errors.
func ValidateRoleTemplateMigration(id, body string, params map[string]string) error {
	if legacyPromptAction.MatchString(body) {
		return fmt.Errorf("agent %q: legacy {{ prompt }} is not supported; use {{ .Prompt }} or {{ .Input }}", id)
	}

	names := make([]string, 0, len(params))
	for name := range params {
		names = append(names, name)
	}

	sort.Strings(names)

	for _, name := range names {
		pattern := regexp.MustCompile(`{{-?\s*` + regexp.QuoteMeta(name) + `\s*-?}}`)
		if pattern.MatchString(body) {
			return fmt.Errorf("agent %q: legacy flat parameter action {{ %s }} is not supported; use {{ index .Params %q }}", id, name, name)
		}
	}

	return nil
}

func primaryInsertions(root *parse.ListNode) (int, int) {
	primary := 0
	references := 0

	for _, node := range root.Nodes {
		action, ok := node.(*parse.ActionNode)
		if ok && barePrimaryAction(action) {
			primary++
		}

		references += primaryReferences(node)
	}

	return primary, references
}

func barePrimaryAction(action *parse.ActionNode) bool {
	if action == nil || action.Pipe == nil || len(action.Pipe.Decl) != 0 || len(action.Pipe.Cmds) != 1 {
		return false
	}

	command := action.Pipe.Cmds[0]
	if len(command.Args) != 1 {
		return false
	}

	field, ok := command.Args[0].(*parse.FieldNode)

	return ok && len(field.Ident) == 1 && (field.Ident[0] == "Prompt" || field.Ident[0] == "Input")
}

func primaryReferences(node parse.Node) int {
	if node == nil {
		return 0
	}

	switch value := node.(type) {
	case *parse.ListNode:
		return listPrimaryReferences(value)
	case *parse.ActionNode:
		if value == nil {
			return 0
		}

		return primaryReferences(value.Pipe)
	case *parse.PipeNode:
		return pipePrimaryReferences(value)
	case *parse.CommandNode:
		return commandPrimaryReferences(value)
	case *parse.IfNode:
		return branchPrimaryReferences(value.Pipe, value.List, value.ElseList)
	case *parse.RangeNode:
		return branchPrimaryReferences(value.Pipe, value.List, value.ElseList)
	case *parse.WithNode:
		return branchPrimaryReferences(value.Pipe, value.List, value.ElseList)
	case *parse.TemplateNode:
		if value == nil {
			return 0
		}

		return primaryReferences(value.Pipe)
	case *parse.FieldNode:
		if value != nil && len(value.Ident) > 0 && (value.Ident[0] == "Prompt" || value.Ident[0] == "Input") {
			return 1
		}
	}

	return 0
}

func listPrimaryReferences(list *parse.ListNode) int {
	if list == nil {
		return 0
	}

	count := 0
	for _, node := range list.Nodes {
		count += primaryReferences(node)
	}

	return count
}

func pipePrimaryReferences(pipe *parse.PipeNode) int {
	if pipe == nil {
		return 0
	}

	count := 0
	for _, command := range pipe.Cmds {
		count += primaryReferences(command)
	}

	return count
}

func commandPrimaryReferences(command *parse.CommandNode) int {
	if command == nil {
		return 0
	}

	count := 0
	for _, argument := range command.Args {
		count += primaryReferences(argument)
	}

	return count
}

func branchPrimaryReferences(pipe *parse.PipeNode, list, elseList *parse.ListNode) int {
	return primaryReferences(pipe) + primaryReferences(list) + primaryReferences(elseList)
}

func cloneTemplateData(data TemplateData) (TemplateData, error) {
	encoded, err := json.Marshal(data)
	if err != nil {
		return TemplateData{}, fmt.Errorf("snapshot template data: %w", err)
	}

	var cloned TemplateData
	if err := json.Unmarshal(encoded, &cloned); err != nil {
		return TemplateData{}, fmt.Errorf("restore template data snapshot: %w", err)
	}

	return cloned, nil
}

func dateParse(layout, value string) (time.Time, error) {
	if layout == "" || value == "" {
		return time.Time{}, fmt.Errorf("dateParse requires nonempty layout and value")
	}

	parsed, err := time.ParseInLocation(layout, value, time.UTC)
	if err != nil {
		return time.Time{}, err
	}

	return parsed.UTC(), nil
}

func dateFormat(layout string, value time.Time) (string, error) {
	if layout == "" {
		return "", fmt.Errorf("dateFormat requires a nonempty layout")
	}

	return value.UTC().Format(layout), nil
}
