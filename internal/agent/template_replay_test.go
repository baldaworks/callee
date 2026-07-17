package agent

import (
	"fmt"
	"testing"
)

func TestAllowedSprigFunctionsReplayDeterministically(t *testing.T) {
	t.Parallel()

	cases := deterministicSprigCases()
	for _, name := range allowedSprigFunctions {
		body, ok := cases[name]
		if !ok {
			t.Errorf("deterministic replay has no case for %q", name)

			continue
		}

		t.Run(name, func(t *testing.T) {
			t.Parallel()

			parsed, err := ParseTemplate(name, body)
			if err != nil {
				t.Fatalf("ParseTemplate(%q) error: %v", body, err)
			}

			firstOutput, firstErr := RenderTemplate(parsed, TemplateData{})
			secondOutput, secondErr := RenderTemplate(parsed, TemplateData{})

			if name == "fail" {
				if firstErr == nil {
					t.Fatal("fail replay returned nil error")
				}
			} else if firstErr != nil {
				t.Fatalf("RenderTemplate(%q) error: %v", body, firstErr)
			}

			if firstOutput != secondOutput || errorText(firstErr) != errorText(secondErr) {
				t.Errorf("replay mismatch: first=(%q, %v), second=(%q, %v)", firstOutput, firstErr, secondOutput, secondErr)
			}
		})
	}

	if len(cases) != len(allowedSprigFunctions) {
		t.Errorf("deterministic replay cases = %d, allowed functions = %d", len(cases), len(allowedSprigFunctions))
	}
}

func deterministicSprigCases() map[string]string {
	cases := deterministicSprigScalarCases()
	mergeFunctionCases(cases, deterministicSprigStructuredCases())
	addFunctionCases(cases, []string{"squote"}, `{{ %s "hello" }}`)
	addFunctionCases(cases, []string{"append", "mustAppend"}, `{{ %s (list 1) 2 }}`)
	addFunctionCases(cases, []string{"prepend", "mustPrepend"}, `{{ %s (list 2) 1 }}`)
	addFunctionCases(cases, []string{"regexReplaceAll", "mustRegexReplaceAll"}, `{{ %s "a" "banana" "x" }}`)
	addFunctionCases(cases, []string{"regexReplaceAllLiteral", "mustRegexReplaceAllLiteral"}, `{{ %s "a" "banana" "${1}" }}`)
	addFunctionCases(cases, []string{"regexSplit", "mustRegexSplit"}, `{{ %s "a" "banana" -1 }}`)

	return cases
}

func deterministicSprigScalarCases() map[string]string {
	return map[string]string{
		"abbrev":     `{{ abbrev 5 "hello world" }}`,
		"abbrevboth": `{{ abbrevboth 5 10 "123456789012345" }}`,
		"camelcase":  `{{ camelcase "hello world" }}`,
		"cat":        `{{ cat "hello" "world" }}`,
		"contains":   `{{ contains "ell" "hello" }}`,
		"hasPrefix":  `{{ hasPrefix "he" "hello" }}`,
		"hasSuffix":  `{{ hasSuffix "lo" "hello" }}`,
		"indent":     `{{ indent 2 "hello" }}`,
		"initials":   `{{ initials "John Doe" }}`,
		"kebabcase":  `{{ kebabcase "Hello World" }}`,
		"lower":      `{{ lower "HELLO" }}`,
		"nindent":    `{{ nindent 2 "hello" }}`,
		"nospace":    `{{ nospace "a b" }}`,
		"plural":     `{{ plural "one" "many" 2 }}`,
		"quote":      `{{ quote "hello" }}`,
		"repeat":     `{{ repeat 2 "ab" }}`,
		"replace":    `{{ replace "a" "b" "a cat" }}`,
		"snakecase":  `{{ snakecase "Hello World" }}`,
		"substr":     `{{ substr 0 2 "hello" }}`,
		"swapcase":   `{{ swapcase "Hello" }}`,
		"title":      `{{ title "hello world" }}`,
		"trim":       `{{ trim " hello " }}`,
		"trimAll":    `{{ trimAll "$" "$hello$" }}`,
		"trimPrefix": `{{ trimPrefix "pre" "prefix" }}`,
		"trimSuffix": `{{ trimSuffix "fix" "prefix" }}`,
		"trunc":      `{{ trunc 3 "hello" }}`,
		"untitle":    `{{ untitle "Hello" }}`,
		"upper":      `{{ upper "hello" }}`,
		"wrap":       `{{ wrap 2 "abcd" }}`,
		"wrapWith":   `{{ wrapWith 2 "-" "abcd" }}`,
		"default":    `{{ default "fallback" "" }}`,
		"empty":      `{{ empty "" }}`,
		"coalesce":   `{{ coalesce "" "value" }}`,
		"all":        `{{ all true 1 "value" }}`,
		"any":        `{{ any false 0 "value" }}`,
		"ternary":    `{{ ternary "yes" "no" true }}`,
		"fail":       `{{ fail "expected deterministic failure" }}`,
		"atoi":       `{{ atoi "7" }}`,
		"int":        `{{ int "7" }}`,
		"int64":      `{{ int64 "7" }}`,
		"float64":    `{{ float64 "7.5" }}`,
		"toDecimal":  `{{ toDecimal "0777" }}`,
		"toString":   `{{ toString 7 }}`,
		"toStrings":  `{{ toStrings (list 1 2) }}`,
		"add":        `{{ add 1 2 3 }}`,
		"add1":       `{{ add1 1 }}`,
		"sub":        `{{ sub 5 2 }}`,
		"div":        `{{ div 6 2 }}`,
		"mod":        `{{ mod 5 2 }}`,
		"mul":        `{{ mul 2 3 }}`,
		"addf":       `{{ addf 1.5 2.5 }}`,
		"add1f":      `{{ add1f 1.5 }}`,
		"subf":       `{{ subf 5.5 2.0 }}`,
		"divf":       `{{ divf 5.0 2.0 }}`,
		"mulf":       `{{ mulf 2.5 2.0 }}`,
		"max":        `{{ max 1 3 2 }}`,
		"min":        `{{ min 1 3 2 }}`,
		"maxf":       `{{ maxf 1.5 3.5 2.5 }}`,
		"minf":       `{{ minf 1.5 3.5 2.5 }}`,
		"ceil":       `{{ ceil 1.2 }}`,
		"floor":      `{{ floor 1.8 }}`,
		"round":      `{{ round 1.234 2 }}`,
	}
}

func deterministicSprigStructuredCases() map[string]string {
	return map[string]string{
		"list":             `{{ list 1 2 3 }}`,
		"dict":             `{{ dict "a" 1 "b" 2 }}`,
		"get":              `{{ get (dict "a" 1) "a" }}`,
		"hasKey":           `{{ hasKey (dict "a" 1) "a" }}`,
		"dig":              `{{ dig "a" "fallback" (dict "a" "value") }}`,
		"pick":             `{{ pick (dict "a" 1 "b" 2) "a" }}`,
		"omit":             `{{ omit (dict "a" 1 "b" 2) "b" }}`,
		"pluck":            `{{ pluck "a" (dict "a" 1) (dict "a" 2) }}`,
		"concat":           `{{ concat (list 1) (list 2) }}`,
		"first":            `{{ first (list 1 2) }}`,
		"mustFirst":        `{{ mustFirst (list 1 2) }}`,
		"rest":             `{{ rest (list 1 2) }}`,
		"mustRest":         `{{ mustRest (list 1 2) }}`,
		"last":             `{{ last (list 1 2) }}`,
		"mustLast":         `{{ mustLast (list 1 2) }}`,
		"initial":          `{{ initial (list 1 2) }}`,
		"mustInitial":      `{{ mustInitial (list 1 2) }}`,
		"reverse":          `{{ reverse (list 1 2) }}`,
		"mustReverse":      `{{ mustReverse (list 1 2) }}`,
		"compact":          `{{ compact (list "" 1 nil 2) }}`,
		"mustCompact":      `{{ mustCompact (list "" 1 nil 2) }}`,
		"uniq":             `{{ uniq (list 1 1 2) }}`,
		"mustUniq":         `{{ mustUniq (list 1 1 2) }}`,
		"without":          `{{ without (list 1 2 3) 2 }}`,
		"mustWithout":      `{{ mustWithout (list 1 2 3) 2 }}`,
		"has":              `{{ has 2 (list 1 2 3) }}`,
		"mustHas":          `{{ mustHas 2 (list 1 2 3) }}`,
		"slice":            `{{ slice (list 1 2 3) 0 2 }}`,
		"mustSlice":        `{{ mustSlice (list 1 2 3) 0 2 }}`,
		"chunk":            `{{ chunk 2 (list 1 2 3) }}`,
		"mustChunk":        `{{ mustChunk 2 (list 1 2 3) }}`,
		"join":             `{{ join "," (list "a" "b") }}`,
		"sortAlpha":        `{{ sortAlpha (list "b" "a") }}`,
		"deepCopy":         `{{ deepCopy (dict "a" (list 1 2)) }}`,
		"mustDeepCopy":     `{{ mustDeepCopy (dict "a" (list 1 2)) }}`,
		"regexMatch":       `{{ regexMatch "a+" "caa" }}`,
		"mustRegexMatch":   `{{ mustRegexMatch "a+" "caa" }}`,
		"regexFind":        `{{ regexFind "a+" "caa" }}`,
		"mustRegexFind":    `{{ mustRegexFind "a+" "caa" }}`,
		"regexFindAll":     `{{ regexFindAll "a" "banana" -1 }}`,
		"mustRegexFindAll": `{{ mustRegexFindAll "a" "banana" -1 }}`,
		"regexQuoteMeta":   `{{ regexQuoteMeta "a+b" }}`,
		"fromJson":         `{{ fromJson "{\"a\":1}" }}`,
		"mustFromJson":     `{{ mustFromJson "{\"a\":1}" }}`,
		"toJson":           `{{ toJson (dict "a" 1) }}`,
		"mustToJson":       `{{ mustToJson (dict "a" 1) }}`,
		"toPrettyJson":     `{{ toPrettyJson (dict "a" 1) }}`,
		"mustToPrettyJson": `{{ mustToPrettyJson (dict "a" 1) }}`,
		"toRawJson":        `{{ toRawJson (dict "a" "<value>") }}`,
		"mustToRawJson":    `{{ mustToRawJson (dict "a" "<value>") }}`,
		"base":             `{{ base "/tmp/a.txt" }}`,
		"dir":              `{{ dir "/tmp/a.txt" }}`,
		"clean":            `{{ clean "/tmp/../var" }}`,
		"ext":              `{{ ext "/tmp/a.txt" }}`,
		"isAbs":            `{{ isAbs "/tmp/a.txt" }}`,
		"semver":           `{{ semver "1.2.3" }}`,
		"semverCompare":    `{{ semverCompare ">=1.0.0" "1.2.3" }}`,
		"sha1sum":          `{{ sha1sum "value" }}`,
		"sha256sum":        `{{ sha256sum "value" }}`,
		"sha512sum":        `{{ sha512sum "value" }}`,
		"adler32sum":       `{{ adler32sum "value" }}`,
		"b32enc":           `{{ b32enc "value" }}`,
		"b32dec":           `{{ b32dec (b32enc "value") }}`,
		"b64enc":           `{{ b64enc "value" }}`,
		"b64dec":           `{{ b64dec (b64enc "value") }}`,
	}
}

func mergeFunctionCases(destination, source map[string]string) {
	for name, body := range source {
		destination[name] = body
	}
}

func addFunctionCases(cases map[string]string, names []string, format string) {
	for _, name := range names {
		cases[name] = fmt.Sprintf(format, name)
	}
}

func errorText(err error) string {
	if err == nil {
		return ""
	}

	return err.Error()
}
