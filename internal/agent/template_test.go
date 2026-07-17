package agent

import (
	"reflect"
	"sort"
	"strings"
	"testing"
	"text/template"
)

func TestValidateRoleTemplate(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		body    string
		wantErr bool
	}{
		{name: "prompt", body: "Task:\n{{ .Prompt }}"},
		{name: "input", body: "Task:\n{{ .Input }}"},
		{name: "missing", body: "Task", wantErr: true},
		{name: "repeated", body: "{{ .Input }} {{ .Input }}", wantErr: true},
		{name: "mixed", body: "{{ .Prompt }} {{ .Input }}", wantErr: true},
		{name: "conditional", body: "{{ if .State.ok }}{{ .Input }}{{ end }}", wantErr: true},
		{name: "pipeline", body: "{{ .Input | upper }}", wantErr: true},
		{name: "defined duplicate", body: `{{ .Input }}{{ define "extra" }}{{ .Input }}{{ end }}`, wantErr: true},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			err := ValidateRoleTemplate("worker", test.body)
			if (err != nil) != test.wantErr {
				t.Errorf("ValidateRoleTemplate() error = %v, wantErr %t", err, test.wantErr)
			}
		})
	}
}

func TestFuncMapIsExactPositiveList(t *testing.T) {
	t.Parallel()

	const normativeSprigFunctions = `
abbrev abbrevboth camelcase cat contains hasPrefix hasSuffix indent initials
kebabcase lower nindent nospace plural quote repeat replace snakecase squote
substr swapcase title trim trimAll trimPrefix trimSuffix trunc untitle upper
wrap wrapWith
default empty coalesce all any ternary fail
atoi int int64 float64 toDecimal toString toStrings
add add1 sub div mod mul addf add1f subf divf mulf max min maxf minf ceil
floor round
list dict get hasKey dig pick omit pluck append mustAppend prepend mustPrepend
concat first mustFirst rest mustRest last mustLast initial mustInitial reverse
mustReverse compact mustCompact uniq mustUniq without mustWithout has mustHas
slice mustSlice chunk mustChunk join sortAlpha deepCopy mustDeepCopy
regexMatch mustRegexMatch regexFind mustRegexFind regexFindAll mustRegexFindAll
regexReplaceAll mustRegexReplaceAll regexReplaceAllLiteral
mustRegexReplaceAllLiteral regexSplit mustRegexSplit regexQuoteMeta
fromJson mustFromJson toJson mustToJson toPrettyJson mustToPrettyJson
toRawJson mustToRawJson
base dir clean ext isAbs
semver semverCompare
sha1sum sha256sum sha512sum adler32sum b32enc b32dec b64enc b64dec
`

	wantSprig := strings.Fields(normativeSprigFunctions)
	sort.Strings(wantSprig)

	configured := append([]string(nil), allowedSprigFunctions...)
	sort.Strings(configured)

	if !reflect.DeepEqual(configured, wantSprig) {
		t.Errorf("allowedSprigFunctions = %v, want normative set %v", configured, wantSprig)
	}

	got := make([]string, 0, len(FuncMap()))
	for name := range FuncMap() {
		got = append(got, name)
	}

	sort.Strings(got)

	want := append(append([]string(nil), wantSprig...), "dateFormat", "dateParse")
	sort.Strings(want)

	if !reflect.DeepEqual(got, want) {
		t.Errorf("FuncMap() names = %v, want %v", got, want)
	}

	for _, forbidden := range []string{
		"env", "expandenv", "getHostByName", "now", "ago", "date", "randInt", "uuidv4", "shuffle",
		"genCA", "encryptAES", "osBase", "set", "unset", "merge", "mergeOverwrite", "mustMerge", "mustMergeOverwrite",
	} {
		if _, ok := FuncMap()[forbidden]; ok {
			t.Errorf("FuncMap() unexpectedly contains %q", forbidden)
		}

		if _, err := ParseTemplate("forbidden "+forbidden, "{{ "+forbidden+" }}"); err == nil {
			t.Errorf("ParseTemplate() accepted forbidden function %q", forbidden)
		}
	}
}

func TestDateFunctionsRejectInvalidExplicitInput(t *testing.T) {
	t.Parallel()

	tests := []string{
		`{{ dateParse "" "2026-07-17" }}`,
		`{{ dateParse "2006-01-02" "" }}`,
		`{{ dateParse "2006-01-02" "not-a-date" }}`,
		`{{ "2026-07-17" | dateParse "2006-01-02" | dateFormat "" }}`,
	}

	for _, body := range tests {
		parsed, err := ParseTemplate("invalid date", body)
		if err != nil {
			t.Fatalf("ParseTemplate(%q) error: %v", body, err)
		}

		if _, err := RenderTemplate(parsed, TemplateData{}); err == nil {
			t.Errorf("RenderTemplate(%q) error = nil, want invalid date error", body)
		}
	}
}

func TestRenderTemplateUsesSnapshotAndUTCDateFunctions(t *testing.T) {
	t.Parallel()

	parsed, err := ParseTemplate("test", `{{ .Input }} {{ default "missing" .State.absent }} {{ "2026-07-17" | dateParse "2006-01-02" | dateFormat "2006-01-02" }}`)
	if err != nil {
		t.Fatalf("ParseTemplate() error: %v", err)
	}

	state := map[string]any{"nested": map[string]any{"value": "kept"}}

	got, err := RenderTemplate(parsed, TemplateData{Input: "goal", State: state})
	if err != nil {
		t.Fatalf("RenderTemplate() error: %v", err)
	}

	if want := "goal missing 2026-07-17"; got != want {
		t.Errorf("RenderTemplate() = %q, want %q", got, want)
	}

	if strings.TrimSpace(state["nested"].(map[string]any)["value"].(string)) != "kept" {
		t.Errorf("RenderTemplate() mutated input state: %v", state)
	}
}

func TestTemplateSurfaceNamespaces(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		parse   func(string, string) (*template.Template, error)
		body    string
		wantErr string
	}{
		{name: "common params", parse: ParseTemplate, body: `{{ .Params.focus }}`},
		{name: "common output", parse: ParseTemplate, body: `{{ .Output }}`, wantErr: ".Output"},
		{name: "restricted params", parse: ParseRestrictedTemplate, body: `{{ .Params.focus }}`, wantErr: ".Params"},
		{name: "restricted output", parse: ParseRestrictedTemplate, body: `{{ .Output }}`, wantErr: ".Output"},
		{name: "output params and output", parse: ParseOutputTemplate, body: `{{ .Output }}{{ .Params.focus }}`},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			_, err := test.parse(test.name, test.body)
			switch {
			case test.wantErr == "" && err != nil:
				t.Errorf("parse error = %v, want nil", err)
			case test.wantErr != "" && (err == nil || !strings.Contains(err.Error(), test.wantErr)):
				t.Errorf("parse error = %v, want containing %q", err, test.wantErr)
			}
		})
	}
}
