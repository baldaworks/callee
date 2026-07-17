package agent

import (
	"bytes"
	"fmt"
	"io"
	"path/filepath"
	"strings"
	"unicode/utf8"

	"gopkg.in/yaml.v3"
)

const markdownDelimiter = "---"

// SupportsFile reports whether path has a supported lowercase agent file
// extension.
func SupportsFile(path string) bool {
	switch filepath.Ext(path) {
	case ".md", ".yaml", ".yml":
		return true
	default:
		return false
	}
}

// Decode decodes one agent file according to its lowercase extension.
func Decode(id, source string, data []byte) (Resource, error) {
	switch filepath.Ext(source) {
	case ".md":
		return DecodeMarkdown(id, source, data)
	case ".yaml", ".yml":
		return DecodeYAML(id, source, data)
	default:
		return Resource{}, fmt.Errorf("agent %q: unsupported file extension %q (want .md, .yaml, or .yml)", id, filepath.Ext(source))
	}
}

// DecodeMarkdown decodes one resource and binds its physical Markdown body to
// spec.body before schema and semantic validation.
func DecodeMarkdown(id, source string, data []byte) (Resource, error) {
	frontmatter, body, bodyStartLine, err := splitMarkdown(data)
	if err != nil {
		return Resource{}, fmt.Errorf("agent %q: %w", id, err)
	}

	var document yaml.Node
	if err := yaml.Unmarshal(frontmatter, &document); err != nil {
		return Resource{}, fmt.Errorf("agent %q: decode YAML frontmatter: %w", id, err)
	}

	if err := validateDocumentDispatch(id, source, &document, 1); err != nil {
		return Resource{}, err
	}

	if bodyNode := nodeAtPath(&document, "spec", "body"); bodyNode != nil {
		return Resource{}, fmt.Errorf("agent %q: spec.body is authored in frontmatter at %s and as the physical Markdown body beginning at %s", id, sourcePosition(source, bodyNode.Line+1, bodyNode.Column), sourcePosition(source, bodyStartLine, 1))
	}

	if !utf8.Valid(body) {
		return Resource{}, fmt.Errorf("agent %q: physical Markdown body must be valid UTF-8", id)
	}

	var resource Resource

	decoder := yaml.NewDecoder(bytes.NewReader(frontmatter))
	decoder.KnownFields(true)

	if err := decoder.Decode(&resource); err != nil {
		message := err.Error()
		if strings.Contains(message, " not found in type ") {
			message = strings.Replace(message, "field ", "unknown frontmatter field ", 1)
		}

		return Resource{}, fmt.Errorf("agent %q: %s", id, message)
	}

	resource.ID = id
	resource.Source = source
	resource.Spec.Body = string(body)

	var raw map[string]any
	if err := document.Decode(&raw); err != nil {
		return Resource{}, fmt.Errorf("agent %q: decode canonical frontmatter: %w", id, err)
	}

	rawSpec, ok := raw["spec"].(map[string]any)
	if !ok {
		return Resource{}, fmt.Errorf("agent %q: spec must be an object", id)
	}

	rawSpec["body"] = string(body)

	if err := validateSchemaDocument(raw); err != nil {
		return Resource{}, fmt.Errorf("agent %q: validate schema: %w", id, err)
	}

	if err := resource.Validate(); err != nil {
		return Resource{}, err
	}

	return resource, nil
}

// DecodeYAML decodes one complete canonical resource from a single YAML
// document. Unlike Markdown, YAML must author spec.body inline.
func DecodeYAML(id, source string, data []byte) (Resource, error) {
	if !utf8.Valid(data) {
		return Resource{}, fmt.Errorf("agent %q: YAML document must be valid UTF-8", id)
	}

	document, err := decodeSingleYAMLDocument(id, data)
	if err != nil {
		return Resource{}, err
	}

	if err := validateDocumentDispatch(id, source, &document, 0); err != nil {
		return Resource{}, err
	}

	var resource Resource

	decoder := yaml.NewDecoder(bytes.NewReader(data))
	decoder.KnownFields(true)

	if err := decoder.Decode(&resource); err != nil {
		message := err.Error()
		if strings.Contains(message, " not found in type ") {
			message = strings.Replace(message, "field ", "unknown YAML field ", 1)
		}

		return Resource{}, fmt.Errorf("agent %q: %s", id, message)
	}

	resource.ID = id
	resource.Source = source

	var raw map[string]any
	if err := document.Decode(&raw); err != nil {
		return Resource{}, fmt.Errorf("agent %q: decode canonical YAML object: %w", id, err)
	}

	if err := validateSchemaDocument(raw); err != nil {
		return Resource{}, fmt.Errorf("agent %q: validate schema: %w", id, err)
	}

	if err := resource.Validate(); err != nil {
		return Resource{}, err
	}

	return resource, nil
}

func decodeSingleYAMLDocument(id string, data []byte) (yaml.Node, error) {
	decoder := yaml.NewDecoder(bytes.NewReader(data))

	var document yaml.Node
	if err := decoder.Decode(&document); err != nil {
		if err == io.EOF {
			return yaml.Node{}, fmt.Errorf("agent %q: YAML file must contain one document", id)
		}

		return yaml.Node{}, fmt.Errorf("agent %q: decode YAML document: %w", id, err)
	}

	var extra yaml.Node
	if err := decoder.Decode(&extra); err != io.EOF {
		if err == nil {
			return yaml.Node{}, fmt.Errorf("agent %q: YAML file must contain exactly one document", id)
		}

		return yaml.Node{}, fmt.Errorf("agent %q: decode trailing YAML document: %w", id, err)
	}

	return document, nil
}

// EncodeMarkdown encodes a canonical resource with spec.body removed from
// frontmatter and emitted byte-for-byte after the closing delimiter.
func EncodeMarkdown(resource Resource) ([]byte, error) {
	if err := resource.Validate(); err != nil {
		return nil, err
	}

	body := resource.Spec.Body
	resource.Spec.Body = ""

	frontmatter, err := yaml.Marshal(resource)
	if err != nil {
		return nil, fmt.Errorf("marshal agent %q frontmatter: %w", resource.ID, err)
	}

	var encoded bytes.Buffer
	encoded.WriteString(markdownDelimiter)
	encoded.WriteByte('\n')
	encoded.Write(frontmatter)
	encoded.WriteString(markdownDelimiter)
	encoded.WriteByte('\n')
	encoded.WriteString(body)

	return encoded.Bytes(), nil
}

func splitMarkdown(data []byte) ([]byte, []byte, int, error) {
	firstEnd, firstNext := lineEnd(data, 0)
	if firstEnd < 0 || string(trimLineEnding(data[:firstEnd])) != markdownDelimiter {
		return nil, nil, 0, fmt.Errorf("missing YAML frontmatter opening delimiter")
	}

	for start := firstNext; start <= len(data); {
		end, next := lineEnd(data, start)
		if end < 0 {
			break
		}

		if string(trimLineEnding(data[start:end])) == markdownDelimiter {
			bodyStartLine := bytes.Count(data[:next], []byte{'\n'}) + 1

			return data[firstNext:start], data[next:], bodyStartLine, nil
		}

		if next <= start {
			break
		}

		start = next
	}

	return nil, nil, 0, fmt.Errorf("missing YAML frontmatter closing delimiter")
}

func validateDocumentDispatch(id, source string, document *yaml.Node, lineOffset int) error {
	firstValueLine := 1 + lineOffset

	version := nodeAtPath(document, "apiVersion")
	if version == nil {
		return fmt.Errorf("agent %q: %s: missing apiVersion; supported versions: %q", id, sourcePosition(source, firstValueLine, 1), APIVersion)
	}

	if version.Kind != yaml.ScalarNode || version.Value != APIVersion {
		return fmt.Errorf("agent %q: %s: unsupported apiVersion %q; supported versions: %q", id, sourcePosition(source, version.Line+lineOffset, version.Column), version.Value, APIVersion)
	}

	kind := nodeAtPath(document, "kind")
	if kind == nil {
		return fmt.Errorf("agent %q: %s: missing kind; supported kinds: Role, Sequential, Loop", id, sourcePosition(source, firstValueLine, 1))
	}

	switch Kind(kind.Value) {
	case RoleKind, SequentialKind, LoopKind:
		return nil
	default:
		return fmt.Errorf("agent %q: %s: unsupported kind %q; supported kinds: Role, Sequential, Loop", id, sourcePosition(source, kind.Line+lineOffset, kind.Column), kind.Value)
	}
}

func sourcePosition(source string, line, column int) string {
	if source == "" {
		return fmt.Sprintf("line %d, column %d", line, column)
	}

	return fmt.Sprintf("%s:%d:%d", source, line, column)
}

func lineEnd(data []byte, start int) (int, int) {
	if start > len(data) {
		return -1, -1
	}

	if index := bytes.IndexByte(data[start:], '\n'); index >= 0 {
		end := start + index + 1

		return end, end
	}

	if start < len(data) {
		return len(data), len(data)
	}

	return -1, -1
}

func trimLineEnding(line []byte) []byte {
	line = bytes.TrimSuffix(line, []byte{'\n'})
	line = bytes.TrimSuffix(line, []byte{'\r'})

	return line
}

func nodeAtPath(document *yaml.Node, path ...string) *yaml.Node {
	if document == nil || len(document.Content) == 0 {
		return nil
	}

	current := document.Content[0]
	for _, name := range path {
		if current.Kind != yaml.MappingNode {
			return nil
		}

		var next *yaml.Node

		for index := 0; index+1 < len(current.Content); index += 2 {
			if current.Content[index].Value == name {
				next = current.Content[index+1]

				break
			}
		}

		if next == nil {
			return nil
		}

		current = next
	}

	return current
}
