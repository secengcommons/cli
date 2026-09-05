package cli

import (
	"encoding/json"
	"errors"
	"reflect"
	"slices"
	"strconv"
	"strings"
	"sync"
	"testing"
)

func TestParseStandardForms(t *testing.T) {
	parser := parserFixture(t)
	for _, arguments := range [][]string{
		{"--root", "repository", "static"},
		{"--root=repository", "static"},
		{"--r=repository", "static"},
		{"-root", "repository", "static"},
		{"-root=repository", "static"},
		{"-r", "repository", "static"},
		{"-r=repository", "static"},
		{"static", "--root", "repository"},
		{"static", "--root=repository"},
		{"static", "-root", "repository"},
		{"static", "-r", "repository"},
	} {
		invocation, err := parser.Parse(arguments)
		if err != nil || invocation.Action() != ActionRun || invocation.Command() != "static" || !invocation.IsSet("root") {
			t.Fatalf("Parse(%q) = (%#v, %v)", arguments, invocation, err)
		}
		if root, found := invocation.String("root"); !found || root != "repository" {
			t.Fatalf("root = (%q, %t)", root, found)
		}
	}
}

func TestOptionValuesUseCanonicalNames(t *testing.T) {
	parser := parserFixture(t)
	invocation, err := parser.Parse([]string{"--root", "-repository", "static"})
	if err != nil {
		t.Fatal(err)
	}
	if root, found := invocation.String("root"); !found || root != "-repository" {
		t.Fatalf("option-looking value = (%q, %t)", root, found)
	}
	if _, found := invocation.Bool("root"); found {
		t.Fatal("string option was returned as boolean")
	}
	if _, found := invocation.String("r"); found || invocation.IsSet("r") {
		t.Fatal("short alias was returned as a canonical option name")
	}
}

func FuzzParse(fuzz *testing.F) {
	for _, seed := range []string{
		"static", "--root\x00repository\x00static", "inspect\x00--format=json\x00file", "help\x00inspect", "--version",
	} {
		fuzz.Add(seed)
	}
	parser, err := New(validDefinition())
	if err != nil {
		fuzz.Fatal(err)
	}
	fuzz.Fuzz(func(t *testing.T, source string) {
		if len(source) > MaxInputBytes {
			return
		}
		checkFuzzParse(t, parser, strings.Split(source, "\x00"))
	})
}

func FuzzRootParse(fuzz *testing.F) {
	for _, seed := range []string{"README.md", "--profile\x00strict.json\x00README.md", "docs", "--\x00-help"} {
		fuzz.Add(seed)
	}
	parser, err := New(Definition{
		Name:      "checker",
		Version:   true,
		Options:   []Option{{Name: "profile", Short: "p", Placeholder: "FILE", Summary: "Configuration profile", Kind: ValueString}},
		Commands:  []Command{{Name: "docs", Summary: "Check documentation"}},
		Arguments: Arguments{Name: "PATH", Minimum: 1, Maximum: 8},
	})
	if err != nil {
		fuzz.Fatal(err)
	}
	fuzz.Fuzz(func(t *testing.T, source string) {
		if len(source) > MaxInputBytes {
			return
		}
		checkFuzzParse(t, parser, strings.Split(source, "\x00"))
	})
}

func FuzzDefinition(fuzz *testing.F) {
	seed, err := json.Marshal(validDefinition())
	if err != nil {
		fuzz.Fatal(err)
	}
	fuzz.Add(seed)
	fuzz.Fuzz(func(t *testing.T, source []byte) {
		if len(source) > MaxDefinitionBytes {
			return
		}
		var definition Definition
		if err := json.Unmarshal(source, &definition); err != nil {
			return
		}
		parser, err := New(definition)
		if err != nil {
			if !errors.Is(err, ErrDefinition) {
				t.Fatalf("definition error = %v", err)
			}
			return
		}
		if _, err = parser.Help(""); err != nil {
			t.Fatalf("admitted help = %v", err)
		}
		if _, err = parser.Parse(nil); err != nil {
			t.Fatalf("admitted parse = %v", err)
		}
		for _, command := range definition.Commands {
			if _, err = parser.Help(command.Name); err != nil {
				t.Fatalf("admitted command help %q = %v", command.Name, err)
			}
			if _, err = parser.Parse([]string{command.Name, "--help"}); err != nil {
				t.Fatalf("admitted command parse %q = %v", command.Name, err)
			}
		}
	})
}

func checkFuzzParse(t *testing.T, parser Parser, arguments []string) {
	t.Helper()
	first, firstErr := parser.Parse(arguments)
	second, secondErr := parser.Parse(arguments)
	if !reflect.DeepEqual(first, second) || (firstErr == nil) != (secondErr == nil) ||
		firstErr != nil && firstErr.Error() != secondErr.Error() {
		t.Fatalf("nondeterministic parse = (%#v, %v) then (%#v, %v)", first, firstErr, second, secondErr)
	}
	if firstErr != nil {
		if !errors.Is(firstErr, ErrInvocation) {
			t.Fatalf("parse error = %v", firstErr)
		}
		return
	}
	if first.Action() == ActionHelp {
		if _, helpErr := parser.Help(first.Command()); helpErr != nil {
			t.Fatalf("help action = %v", helpErr)
		}
		return
	}
	if first.Action() != ActionRun && first.Action() != ActionVersion {
		t.Fatalf("action = %d", first.Action())
	}
}

func TestParseCommandOptionsAndArguments(t *testing.T) {
	parser := parserFixture(t)
	invocation, err := parser.Parse([]string{"--quiet", "inspect", "--format", "json", "--strict", "one", "two"})
	if err != nil {
		t.Fatal(err)
	}
	if invocation.Action() != ActionRun || invocation.Command() != "inspect" {
		t.Fatalf("invocation = (%#v, %v)", invocation, err)
	}
	for _, name := range []string{"quiet", "format", "strict"} {
		if !invocation.IsSet(name) {
			t.Fatalf("option %q is not set", name)
		}
	}
	if format, found := invocation.String("format"); !found || format != "json" {
		t.Fatalf("format = (%q, %t)", format, found)
	}
	if strict, found := invocation.Bool("strict"); !found || !strict {
		t.Fatalf("strict = (%t, %t)", strict, found)
	}
}

func TestParseRootArguments(t *testing.T) {
	parser := rootParserFixture(t)
	invocation, err := parser.Parse([]string{"--profile", "strict.json", "README.md", "docs/"})
	if err != nil || invocation.Action() != ActionRun || invocation.Command() != "" ||
		strings.Join(invocation.Arguments(), ",") != "README.md,docs/" {
		t.Fatalf("root invocation = (%#v, %v)", invocation, err)
	}
	if profile, found := invocation.String("profile"); !found || profile != "strict.json" {
		t.Fatalf("profile = (%q, %t)", profile, found)
	}
	invocation, err = parser.Parse(nil)
	if err != nil || invocation.Action() != ActionHelp {
		t.Fatalf("empty root invocation = (%#v, %v)", invocation, err)
	}
}

func TestRootActionCommandPrecedenceAndBounds(t *testing.T) {
	parser := rootParserFixture(t)
	invocation, err := parser.Parse([]string{"docs"})
	if err != nil || invocation.Command() != "docs" {
		t.Fatalf("named command = (%#v, %v)", invocation, err)
	}
	if _, err = parser.Parse([]string{"one", "two", "three"}); !errors.Is(err, ErrInvocation) {
		t.Fatalf("root argument bound = %v", err)
	}
	invocation, err = parser.Parse([]string{"--", "README.md", "--profile"})
	if err != nil || !reflect.DeepEqual(invocation.Arguments(), []string{"README.md", "--profile"}) || invocation.IsSet("profile") {
		t.Fatalf("root terminator = (%#v, %v)", invocation, err)
	}
	invocation, err = parser.Parse([]string{"-"})
	if err != nil || !reflect.DeepEqual(invocation.Arguments(), []string{"-"}) {
		t.Fatalf("root dash = (%#v, %v)", invocation, err)
	}
}

func TestRootTerminatorForcesRootArguments(t *testing.T) {
	parser := rootParserFixture(t)
	for _, arguments := range [][]string{
		{"--", "docs"},
		{"--", "help"},
		{"--", "version"},
		{"--", "v"},
		{"--", "-value"},
	} {
		invocation, err := parser.Parse(arguments)
		if err != nil || invocation.Action() != ActionRun || invocation.Command() != "" ||
			!reflect.DeepEqual(invocation.Arguments(), arguments[1:]) {
			t.Fatalf("Parse(%q) = (%#v, %v)", arguments, invocation, err)
		}
	}
	invocation, err := parser.Parse([]string{"docs"})
	if err != nil || invocation.Action() != ActionRun || invocation.Command() != "docs" {
		t.Fatalf("command without terminator = (%#v, %v)", invocation, err)
	}
	commandParser := parserFixture(t)
	invocation, err = commandParser.Parse([]string{"--", "static"})
	if err != nil || invocation.Action() != ActionRun || invocation.Command() != "static" {
		t.Fatalf("command-only terminator = (%#v, %v)", invocation, err)
	}
}

func TestParseRootOnlyDefinition(t *testing.T) {
	parser, err := New(Definition{Name: "check", Arguments: Arguments{Name: "PATH", Minimum: 1, Maximum: 1}})
	if err != nil {
		t.Fatal(err)
	}
	invocation, err := parser.Parse([]string{"README.md"})
	if err != nil || invocation.Action() != ActionRun || invocation.Command() != "" ||
		!reflect.DeepEqual(invocation.Arguments(), []string{"README.md"}) {
		t.Fatalf("root-only invocation = (%#v, %v)", invocation, err)
	}
	help, err := parser.Help("")
	if err != nil || !strings.Contains(string(help), "  check [flags] PATH\n  check [flags] [command]\n") {
		t.Fatalf("root-only help = (%q, %v)", help, err)
	}
}

func TestOptionDefaultsRemainDistinctFromExplicitValues(t *testing.T) {
	parser := parserFixture(t)
	invocation, err := parser.Parse([]string{"static"})
	if err != nil {
		t.Fatal(err)
	}
	if root, found := invocation.String("root"); !found || root != "." || invocation.IsSet("root") {
		t.Fatalf("root default = (%q, %t, %t)", root, found, invocation.IsSet("root"))
	}
	if quiet, found := invocation.Bool("quiet"); !found || quiet || invocation.IsSet("quiet") {
		t.Fatalf("quiet default = (%t, %t, %t)", quiet, found, invocation.IsSet("quiet"))
	}
	invocation, err = parser.Parse([]string{"--quiet=false", "static"})
	if err != nil {
		t.Fatal(err)
	}
	if quiet, found := invocation.Bool("quiet"); !found || quiet || !invocation.IsSet("quiet") {
		t.Fatalf("explicit quiet = (%t, %t, %t)", quiet, found, invocation.IsSet("quiet"))
	}
}

func TestEmptyOptionNameDoesNotMatchAbsentShortAlias(t *testing.T) {
	parser, err := New(Definition{
		Name: "value", Options: []Option{{Name: "output", Placeholder: "FILE", Summary: "Output file", Kind: ValueString}},
		Commands: []Command{{Name: "run", Summary: "Run value"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	invocation, err := parser.Parse([]string{"run"})
	if err != nil {
		t.Fatal(err)
	}
	if _, found := invocation.String(""); found || invocation.IsSet("") {
		t.Fatal("empty option name matched an absent short alias")
	}
	if _, found := invocation.String("missing"); found || invocation.IsSet("missing") {
		t.Fatal("missing option was returned")
	}
	if _, found := invocation.Bool("output"); found {
		t.Fatal("string option was returned as boolean")
	}
}

func TestHelpAndVersionDoNotRetainOptionValues(t *testing.T) {
	parser := parserFixture(t)
	for _, arguments := range [][]string{nil, {"help"}, {"version"}, {"--root", "repository"}} {
		invocation, err := parser.Parse(arguments)
		if err != nil || invocation.Action() == ActionRun {
			t.Fatalf("Parse(%q) = (%#v, %v)", arguments, invocation, err)
		}
		if _, found := invocation.String("root"); found || invocation.IsSet("root") {
			t.Fatalf("Parse(%q) retained option values", arguments)
		}
		if invocation.Arguments() != nil || invocation.ArgumentCount() != 0 {
			t.Fatalf("Parse(%q) retained arguments", arguments)
		}
	}
}

func TestOptionOverrideSpill(t *testing.T) {
	parser, err := New(Definition{
		Name: "value",
		Options: []Option{
			{Name: "alpha", Summary: "Select alpha", Kind: ValueBool},
			{Name: "bravo", Summary: "Select bravo", Kind: ValueBool},
			{Name: "charlie", Summary: "Select charlie", Kind: ValueBool},
			{Name: "delta", Summary: "Select delta", Kind: ValueBool},
		},
		Commands: []Command{{Name: "run", Summary: "Run value"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	invocation, err := parser.Parse([]string{"--alpha", "--bravo", "run", "--charlie"})
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"alpha", "bravo", "charlie"} {
		if value, found := invocation.Bool(name); !found || !value || !invocation.IsSet(name) {
			t.Fatalf("override %q = (%t, %t)", name, value, found)
		}
	}
	if invocation.IsSet("delta") {
		t.Fatal("unset spilled option was reported as set")
	}
}

func TestInvocationDoesNotBorrowInputSlice(t *testing.T) {
	parser := parserFixture(t)
	source := []string{"inspect", "--format", "json", "one", "two"}
	invocation, err := parser.Parse(source)
	if err != nil {
		t.Fatal(err)
	}
	for index := range source {
		source[index] = "changed"
	}
	format, formatFound := invocation.String("format")
	if !formatFound || format != "json" {
		t.Fatal("invocation borrowed the input option value")
	}
	if first, found := invocation.Argument(0); !found || first != "one" {
		t.Fatal("invocation borrowed the input slice")
	}
	arguments := invocation.Arguments()
	arguments[0] = "changed"
	if invocation.Arguments()[0] != "one" {
		t.Fatal("invocation arguments are mutable through the result")
	}
}

func TestInvocationArgumentAccessors(t *testing.T) {
	invocation, err := parserFixture(t).Parse([]string{"inspect", "--format", "json", "one", "two"})
	if err != nil {
		t.Fatal(err)
	}
	arguments := invocation.Arguments()
	first, firstFound := invocation.Argument(0)
	second, secondFound := invocation.Argument(1)
	if strings.Join(arguments, ",") != "one,two" || invocation.ArgumentCount() != 2 {
		t.Fatalf("arguments = %#v", arguments)
	}
	if !firstFound || first != "one" || !secondFound || second != "two" {
		t.Fatalf("indexed arguments = (%q, %t, %q, %t)", first, firstFound, second, secondFound)
	}
	if _, found := invocation.Argument(-1); found {
		t.Fatal("negative argument index was accepted")
	}
	if _, found := invocation.Argument(2); found {
		t.Fatal("excess argument index was accepted")
	}
}

func TestSpilledInvocationDoesNotBorrowInputSlice(t *testing.T) {
	parser, err := New(Definition{Name: "check", Arguments: Arguments{Name: "PATH", Minimum: 1, Maximum: 3}})
	if err != nil {
		t.Fatal(err)
	}
	source := []string{"one", "two", "three"}
	invocation, err := parser.Parse(source)
	if err != nil {
		t.Fatal(err)
	}
	source[0] = "changed"
	if first, found := invocation.Argument(0); !found || first != "one" {
		t.Fatal("spilled invocation borrowed the input slice")
	}
}

func TestParseHelpAndVersion(t *testing.T) {
	parser := parserFixture(t)
	for _, test := range []struct {
		arguments []string
		action    Action
		command   string
	}{
		{action: ActionHelp},
		{arguments: []string{"help"}, action: ActionHelp},
		{arguments: []string{"-h"}, action: ActionHelp},
		{arguments: []string{"--help"}, action: ActionHelp},
		{arguments: []string{"help", "static"}, action: ActionHelp, command: "static"},
		{arguments: []string{"help", "help"}, action: ActionHelp, command: "help"},
		{arguments: []string{"help", "version"}, action: ActionHelp, command: "version"},
		{arguments: []string{"--help", "static"}, action: ActionHelp, command: "static"},
		{arguments: []string{"static", "--help"}, action: ActionHelp, command: "static"},
		{arguments: []string{"help", "--help"}, action: ActionHelp, command: "help"},
		{arguments: []string{"version", "--help"}, action: ActionHelp, command: "version"},
		{arguments: []string{"version", "-h"}, action: ActionHelp, command: "version"},
		{arguments: []string{"version"}, action: ActionVersion},
		{arguments: []string{"v"}, action: ActionVersion},
		{arguments: []string{"-v"}, action: ActionVersion},
		{arguments: []string{"--v"}, action: ActionVersion},
		{arguments: []string{"--version"}, action: ActionVersion},
		{arguments: []string{"static", "--version"}, action: ActionVersion},
		{arguments: []string{"static", "-v"}, action: ActionVersion},
		{arguments: []string{"help", "--version"}, action: ActionVersion},
		{arguments: []string{"version", "--version"}, action: ActionVersion},
		{arguments: []string{"help", "v"}, action: ActionHelp, command: "version"},
		{arguments: []string{"v", "--help"}, action: ActionHelp, command: "version"},
		{arguments: []string{"help", "--root", "repository", "static"}, action: ActionHelp, command: "static"},
		{arguments: []string{"version", "--root", "repository"}, action: ActionVersion},
	} {
		invocation, err := parser.Parse(test.arguments)
		if err != nil || invocation.Action() != test.action || invocation.Command() != test.command {
			t.Fatalf("Parse(%q) = (%#v, %v)", test.arguments, invocation, err)
		}
	}
}

func TestParseRejectsInvalidInvocations(t *testing.T) {
	parser := parserFixture(t)
	invalid := [][]string{
		{"unknown"},
		{"-x"},
		{"---root"},
		{"--=root"},
		{"--root"},
		{"--root="},
		{"--root", "one", "--root", "two", "static"},
		{"static", "--root", "one", "-r", "two"},
		{"inspect", "one"},
		{"inspect", "--format", "json"},
		{"inspect", "--format", "json", "one", "two", "three"},
		{"help", "static", "extra"},
		{"help", "unknown"},
		{"help", "--unknown"},
		{"--help=invalid"},
		{"--version=invalid"},
		{"static", "--help", "extra"},
		{"version", "extra"},
		{"--version", "static"},
		{"-h", "--help"},
		{"-v", "--version"},
		{"inspect", "--format", "json", "--strict=invalid", "one"},
		{string([]byte{0xff})},
		{"static", "line\nfeed"},
		{"static", "delete\x7fcharacter"},
		{"static", "direction\u202e"},
	}
	for _, arguments := range invalid {
		if _, err := parser.Parse(arguments); !errors.Is(err, ErrInvocation) {
			t.Fatalf("Parse(%q) error = %v", arguments, err)
		}
	}
	tooMany := make([]string, MaxInputArguments+1)
	if _, err := parser.Parse(tooMany); !errors.Is(err, ErrInvocation) {
		t.Fatalf("argument-count error = %v", err)
	}
	if _, err := parser.Parse([]string{strings.Repeat("x", MaxInputBytes+1)}); !errors.Is(err, ErrInvocation) {
		t.Fatalf("argument-byte error = %v", err)
	}
}

func TestPrintableUnicodeIsRetained(t *testing.T) {
	definition := Definition{
		Name: "check", Summary: "Check café files",
		Options:  []Option{{Name: "label", Placeholder: "TEXT", Summary: "Café label", Kind: ValueString, Default: "café"}},
		Commands: []Command{{Name: "run", Summary: "Run café check", Arguments: Arguments{Name: "TEXT", Minimum: 1, Maximum: 1}}},
	}
	parser, err := New(definition)
	if err != nil {
		t.Fatal(err)
	}
	invocation, err := parser.Parse([]string{"run", "naïve"})
	argument, found := invocation.Argument(0)
	if err != nil || !found || argument != "naïve" {
		t.Fatalf("Unicode argument = (%q, %t, %v)", argument, found, err)
	}
	help, err := parser.Help("")
	if err != nil || !strings.Contains(string(help), "Check café files") || !strings.Contains(string(help), "café") {
		t.Fatalf("Unicode help = (%q, %v)", help, err)
	}
}

func TestParseBooleanForms(t *testing.T) {
	parser := parserFixture(t)
	for _, value := range []string{"1", "t", "T", "true", "TRUE", "True"} {
		invocation, err := parser.Parse([]string{"static", "--quiet=" + value})
		if enabled, found := invocation.Bool("quiet"); err != nil || !found || !enabled {
			t.Fatalf("true value %q = (%t, %t, %v)", value, enabled, found, err)
		}
	}
	for _, value := range []string{"0", "f", "F", "false", "FALSE", "False"} {
		invocation, err := parser.Parse([]string{"static", "--quiet=" + value})
		if enabled, found := invocation.Bool("quiet"); err != nil || !found || enabled {
			t.Fatalf("false value %q = (%t, %t, %v)", value, enabled, found, err)
		}
	}
}

func TestBooleanErrorsDoNotExposeRejectedValues(t *testing.T) {
	parser := parserFixture(t)
	const rejected = "accidentally-sensitive-value"
	_, err := parser.Parse([]string{"static", "--quiet=" + rejected})
	if !errors.Is(err, ErrInvocation) || !strings.Contains(err.Error(), "--quiet") || strings.Contains(err.Error(), rejected) {
		t.Fatalf("boolean error = %v", err)
	}
	diagnostic, diagnosticErr := parser.Diagnostic(err)
	if diagnosticErr != nil || !strings.Contains(string(diagnostic), "--quiet") || strings.Contains(string(diagnostic), rejected) {
		t.Fatalf("boolean diagnostic = (%q, %v)", diagnostic, diagnosticErr)
	}
}

func TestMalformedOptionErrorsDoNotExposeRejectedValues(t *testing.T) {
	parser := parserFixture(t)
	const rejected = "accidentally-sensitive-value"
	_, err := parser.Parse([]string{"--=" + rejected})
	if !errors.Is(err, ErrInvocation) || !strings.Contains(err.Error(), "flag syntax") || strings.Contains(err.Error(), rejected) {
		t.Fatalf("malformed option error = %v", err)
	}
	diagnostic, diagnosticErr := parser.Diagnostic(err)
	if diagnosticErr != nil || !strings.Contains(string(diagnostic), "flag syntax") || strings.Contains(string(diagnostic), rejected) {
		t.Fatalf("malformed option diagnostic = (%q, %v)", diagnostic, diagnosticErr)
	}
}

func TestParseHonoursOptionTerminator(t *testing.T) {
	parser := parserFixture(t)
	invocation, err := parser.Parse([]string{"inspect", "--format", "json", "--", "--strict"})
	if err != nil || strings.Join(invocation.Arguments(), "") != "--strict" || invocation.IsSet("strict") {
		t.Fatalf("terminator = (%#v, %v)", invocation, err)
	}
	invocation, err = parser.Parse([]string{"inspect", "--format", "json", "one", "--strict"})
	if err != nil || strings.Join(invocation.Arguments(), ",") != "one,--strict" || invocation.IsSet("strict") {
		t.Fatalf("positional boundary = (%#v, %v)", invocation, err)
	}
}

func TestInputBoundariesAreReachable(t *testing.T) {
	definition := Definition{
		Name: "value", Summary: "Value",
		Commands: []Command{{Name: "run", Summary: "Run value", Arguments: Arguments{Name: "ARG", Maximum: MaxPositionalArguments}}},
	}
	parser, err := New(definition)
	if err != nil {
		t.Fatal(err)
	}
	arguments := make([]string, MaxInputArguments)
	arguments[0] = "run"
	for index := 1; index < len(arguments); index++ {
		arguments[index] = "x"
	}
	invocation, err := parser.Parse(arguments)
	last, found := invocation.Argument(MaxPositionalArguments - 1)
	if err != nil || len(invocation.Arguments()) != MaxPositionalArguments || !found || last != "x" {
		t.Fatalf("maximum arguments = (%d, %v)", len(invocation.Arguments()), err)
	}

	parser = parserFixture(t)
	prefix := "--root="
	root := strings.Repeat("x", MaxInputBytes-len(prefix)-len("static"))
	if _, err = parser.Parse([]string{prefix + root, "static"}); err != nil {
		t.Fatalf("maximum argument bytes = %v", err)
	}
}

func TestArgumentCountDiagnostics(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		arguments Arguments
		received  int
		want      string
	}{
		{want: `command "run" accepts no arguments`},
		{arguments: Arguments{Minimum: 1, Maximum: 1}, want: `command "run" requires 1 argument but received 0`},
		{arguments: Arguments{Maximum: 2}, received: 3, want: `command "run" accepts at most 2 arguments but received 3`},
		{arguments: Arguments{Minimum: 1, Maximum: 2}, received: 3, want: `command "run" accepts 1 to 2 arguments but received 3`},
	} {
		if got := argumentCountError("run", test.arguments, test.received).Error(); got != test.want {
			t.Fatalf("argument error = %q", got)
		}
	}
}

func TestDefinitionIsCopied(t *testing.T) {
	definition := validDefinition()
	parser, err := New(definition)
	if err != nil {
		t.Fatal(err)
	}
	definition.Name = "changed"
	definition.Options[0].Name = "changed"
	definition.Commands[0].Name = "changed"
	definition.Commands[1].Options[0].Name = "changed"
	invocation, err := parser.Parse([]string{"static", "--root", "repository"})
	if err != nil || invocation.Command() != "static" {
		t.Fatalf("parse after source mutation = (%#v, %v)", invocation, err)
	}
}

func TestParserSupportsConcurrentOperations(t *testing.T) {
	parser := parserFixture(t)
	invocation, err := parser.Parse([]string{"inspect", "--format", "json", "one", "two"})
	if err != nil {
		t.Fatal(err)
	}
	parseErr := invocationError("failure")
	operations := []func(){
		func() { checkConcurrentParse(t, parser) },
		func() { checkConcurrentHelp(t, parser) },
		func() { checkConcurrentDiagnostic(t, parser, parseErr) },
		func() { checkConcurrentGetters(t, invocation) },
	}
	const workers = 32
	var group sync.WaitGroup
	group.Add(workers)
	for worker := range workers {
		operation := operations[worker%len(operations)]
		go func() {
			defer group.Done()
			operation()
		}()
	}
	group.Wait()
}

func checkConcurrentParse(t *testing.T, parser Parser) {
	t.Helper()
	invocation, err := parser.Parse([]string{"static", "--root", "repository"})
	if err != nil || invocation.Command() != "static" {
		t.Errorf("concurrent parse = (%#v, %v)", invocation, err)
	}
}

func checkConcurrentHelp(t *testing.T, parser Parser) {
	t.Helper()
	help, err := parser.Help("inspect")
	if err != nil || !strings.Contains(string(help), "secverify inspect") {
		t.Errorf("concurrent help = (%q, %v)", help, err)
	}
}

func checkConcurrentDiagnostic(t *testing.T, parser Parser, parseErr error) {
	t.Helper()
	diagnostic, err := parser.Diagnostic(parseErr)
	if err != nil || !strings.Contains(string(diagnostic), "Error: failure") {
		t.Errorf("concurrent diagnostic = (%q, %v)", diagnostic, err)
	}
}

func checkConcurrentGetters(t *testing.T, invocation Invocation) {
	t.Helper()
	first, found := invocation.Argument(0)
	format, valueFound := invocation.String("format")
	if invocation.Action() != ActionRun || invocation.Command() != "inspect" || invocation.ArgumentCount() != 2 ||
		!found || first != "one" || !valueFound || format != "json" || !invocation.IsSet("format") {
		t.Error("concurrent invocation getters returned inconsistent values")
	}
}

func TestNewRejectsInvalidDefinitions(t *testing.T) {
	for _, mutate := range []func(*Definition){
		func(value *Definition) { value.Name = "" },
		func(value *Definition) { value.Name = "Invalid" },
		func(value *Definition) { value.Name = "invalid_name" },
		func(value *Definition) { value.Name = "invalid-" },
		func(value *Definition) { value.Name = "invalid--name" },
		func(value *Definition) { value.Summary = "line\nfeed" },
		func(value *Definition) { value.Summary = " Verify repositories" },
		func(value *Definition) { value.Summary = " " },
		func(value *Definition) { value.Summary = "direction\u202e" },
		func(value *Definition) { value.Summary = string([]byte{0xff}) },
		func(value *Definition) { value.Summary = "nul\x00value" },
		func(value *Definition) { value.Commands = nil },
		func(value *Definition) { value.Commands = make([]Command, MaxCommands+1) },
		func(value *Definition) { value.Commands[1].Name = value.Commands[0].Name },
		func(value *Definition) { value.Commands[0].Name = "help" },
		func(value *Definition) { value.Commands[0].Name = "version" },
		func(value *Definition) { value.Commands[0].Name = "v" },
		func(value *Definition) { value.Commands[0].Summary = "" },
		func(value *Definition) { value.Commands[0].Summary = "Run verification " },
		func(value *Definition) { value.Commands[0].Arguments = Arguments{Name: "ARG", Minimum: 2, Maximum: 1} },
		func(value *Definition) { value.Commands[0].Arguments = Arguments{Name: "", Maximum: 1} },
		func(value *Definition) { value.Commands[0].Arguments = Arguments{Name: "-ARG", Maximum: 1} },
		func(value *Definition) { value.Commands[0].Arguments = Arguments{Name: "ARG-", Maximum: 1} },
		func(value *Definition) { value.Commands[0].Arguments = Arguments{Name: "ARG--VALUE", Maximum: 1} },
		func(value *Definition) { value.Arguments = Arguments{Name: "PATH", Maximum: 1} },
		func(value *Definition) { value.Arguments = Arguments{Name: "", Minimum: 1, Maximum: 1} },
		func(value *Definition) { value.Arguments = Arguments{Name: "1PATH", Minimum: 1, Maximum: 1} },
		func(value *Definition) {
			value.Arguments = Arguments{Name: "PATH", Minimum: 1, Maximum: MaxPositionalArguments + 1}
		},
		func(value *Definition) {
			value.Commands[0].Arguments = Arguments{Name: "ARG", Maximum: MaxPositionalArguments + 1}
		},
		func(value *Definition) { value.Options = make([]Option, MaxOptions+1) },
		func(value *Definition) { value.Commands[0].Options = make([]Option, MaxOptions+1) },
		func(value *Definition) { value.Options[1].Name = value.Options[0].Name },
		func(value *Definition) { value.Commands[1].Options[1].Name = value.Commands[1].Options[0].Name },
		func(value *Definition) { value.Options[0].Name = "help" },
		func(value *Definition) { value.Options[0].Name = "h" },
		func(value *Definition) { value.Options[0].Name = "version" },
		func(value *Definition) { value.Options[0].Name = "v" },
		func(value *Definition) { value.Options[0].Short = "h" },
		func(value *Definition) { value.Options[0].Short = "v" },
		func(value *Definition) { value.Options[0].Name = value.Options[0].Short },
		func(value *Definition) { value.Options[0].Short = "rr" },
		func(value *Definition) { value.Options[0].Summary = "" },
		func(value *Definition) { value.Options[0].Summary = " Repository root" },
		func(value *Definition) { value.Options[0].Kind = 0 },
		func(value *Definition) { value.Options[0].Placeholder = "path" },
		func(value *Definition) { value.Options[0].Placeholder = "-PATH" },
		func(value *Definition) {
			value.Options[0].Default = ""
			value.Options[0].Required = true
		},
		func(value *Definition) { value.Commands[1].Options[0].Name = value.Options[0].Name },
		func(value *Definition) { value.Commands[1].Options[0].Short = value.Options[0].Short },
		func(value *Definition) { value.Commands[1].Options[0].Name = value.Options[0].Short },
		func(value *Definition) {
			value.Options[0].Name = "x"
			value.Commands[1].Options[0].Short = "x"
		},
		func(value *Definition) { value.Commands[1].Options[0].Default = "default" },
		func(value *Definition) { value.Commands[1].Options[1].Default = "invalid" },
	} {
		definition := validDefinition()
		mutate(&definition)
		if _, err := New(definition); !errors.Is(err, ErrDefinition) {
			t.Fatalf("definition %#v error = %v", definition, err)
		}
	}
}

func TestVersionNameIsAvailableWithoutBuiltinVersion(t *testing.T) {
	for _, name := range []string{versionName, versionAlias} {
		definition := Definition{
			Name: "value", Summary: "Value", Options: []Option{{Name: name, Summary: "Selected version", Kind: ValueBool}},
			Commands: []Command{{Name: name, Summary: "Run version command"}},
		}
		parser, err := New(definition)
		if err != nil {
			t.Fatal(err)
		}
		invocation, err := parser.Parse([]string{name, "--" + name})
		if err != nil || invocation.Action() != ActionRun || invocation.Command() != name {
			t.Fatalf("version command %q = (%#v, %v)", name, invocation, err)
		}
		if enabled, found := invocation.Bool(name); !found || !enabled {
			t.Fatalf("version option %q = (%t, %t)", name, enabled, found)
		}
	}
}

func TestShortOptionAlphabet(t *testing.T) {
	t.Parallel()
	for _, short := range []string{"a", "Z", "7"} {
		definition := Definition{
			Name: "value", Summary: "Value", Options: []Option{{Name: "selected", Short: short, Summary: "Select value", Kind: ValueBool}},
			Commands: []Command{{Name: "run", Summary: "Run value"}},
		}
		parser, err := New(definition)
		if err != nil {
			t.Fatal(err)
		}
		invocation, err := parser.Parse([]string{"-" + short, "run"})
		if err != nil || !invocation.IsSet("selected") {
			t.Fatalf("short %q = (%#v, %v)", short, invocation, err)
		}
	}
}

func TestDefinitionByteBound(t *testing.T) {
	definition := Definition{Name: "value", Summary: "Value", Commands: make([]Command, MaxCommands)}
	for command := range definition.Commands {
		definition.Commands[command] = Command{
			Name: "command-" + strconv.Itoa(command), Summary: "Command", Options: make([]Option, MaxOptions),
		}
		for option := range definition.Commands[command].Options {
			definition.Commands[command].Options[option] = Option{
				Name: "option-" + strconv.Itoa(option), Placeholder: "VALUE", Summary: strings.Repeat("x", MaxSummaryBytes), Kind: ValueString,
			}
		}
	}
	if definitionBytes(definition) <= MaxDefinitionBytes {
		t.Fatal("oversized definition fixture is not oversized")
	}
	if _, err := New(definition); !errors.Is(err, ErrDefinition) {
		t.Fatalf("oversized definition error = %v", err)
	}
}

func TestDefinitionByteBoundaryIsReachable(t *testing.T) {
	definition := Definition{
		Name: "value", Options: []Option{{Name: "input", Placeholder: "VALUE", Summary: "Input value", Kind: ValueString}},
		Commands: []Command{{Name: "run", Summary: "Run value"}},
	}
	remaining := MaxDefinitionBytes - definitionBytes(definition)
	definition.Options[0].Default = strings.Repeat("x", remaining)
	if definitionBytes(definition) != MaxDefinitionBytes {
		t.Fatal("definition fixture does not reach the byte boundary")
	}
	if _, err := New(definition); err != nil {
		t.Fatalf("maximum definition error = %v", err)
	}
	definition.Options[0].Default += "x"
	if _, err := New(definition); !errors.Is(err, ErrDefinition) {
		t.Fatalf("oversized definition error = %v", err)
	}
}

func TestDefinitionFieldBoundaries(t *testing.T) {
	definition := validDefinition()
	definition.Name = "a" + strings.Repeat("x", MaxNameBytes-1)
	definition.Summary = strings.Repeat("x", MaxSummaryBytes)
	definition.Commands[0].Name = "a" + strings.Repeat("x", MaxNameBytes-1)
	definition.Commands[0].Summary = strings.Repeat("x", MaxSummaryBytes)
	definition.Options[0].Name = "a" + strings.Repeat("x", MaxNameBytes-1)
	definition.Options[0].Summary = strings.Repeat("x", MaxSummaryBytes)
	definition.Options[0].Placeholder = "X-" + strings.Repeat("X", MaxNameBytes-2)
	if _, err := New(definition); err != nil {
		t.Fatalf("maximum fields error = %v", err)
	}
	for _, mutate := range []func(*Definition){
		func(value *Definition) { value.Name += "x" },
		func(value *Definition) { value.Summary += "x" },
		func(value *Definition) { value.Commands[0].Name += "x" },
		func(value *Definition) { value.Commands[0].Summary += "x" },
		func(value *Definition) { value.Options[0].Name += "x" },
		func(value *Definition) { value.Options[0].Summary += "x" },
		func(value *Definition) { value.Options[0].Placeholder += "X" },
	} {
		candidate := definition
		candidate.Options = slices.Clone(definition.Options)
		candidate.Commands = slices.Clone(definition.Commands)
		mutate(&candidate)
		if _, err := New(candidate); !errors.Is(err, ErrDefinition) {
			t.Fatalf("oversized field error = %v", err)
		}
	}
}

func TestDefinitionCountBoundariesAreReachable(t *testing.T) {
	definition := Definition{Name: "value", Commands: make([]Command, MaxCommands), Options: make([]Option, MaxOptions)}
	for index := range definition.Commands {
		definition.Commands[index] = Command{Name: "command-" + strconv.Itoa(index), Summary: "Run command"}
	}
	for index := range definition.Options {
		definition.Options[index] = Option{Name: "option-" + strconv.Itoa(index), Summary: "Select option", Kind: ValueBool}
	}
	definition.Commands[0].Options = make([]Option, MaxOptions)
	for index := range definition.Commands[0].Options {
		definition.Commands[0].Options[index] = Option{Name: "local-" + strconv.Itoa(index), Summary: "Select local option", Kind: ValueBool}
	}
	if _, err := New(definition); err != nil {
		t.Fatalf("maximum counts error = %v", err)
	}
}

func TestZeroParserFailsClosed(t *testing.T) {
	t.Parallel()
	var parser Parser
	if _, err := parser.Parse(nil); !errors.Is(err, ErrDefinition) {
		t.Fatalf("zero Parse error = %v", err)
	}
	if _, err := parser.Help(""); !errors.Is(err, ErrDefinition) {
		t.Fatalf("zero Help error = %v", err)
	}
}

func TestParseErrorPreservesInvocationIdentity(t *testing.T) {
	t.Parallel()
	value := parseError{message: "failure"}
	if value.Error() != "failure" || !errors.Is(value, ErrInvocation) {
		t.Fatalf("parse error = %v", value)
	}
}

func parserFixture(t *testing.T) Parser {
	t.Helper()
	parser, err := New(validDefinition())
	if err != nil {
		t.Fatal(err)
	}
	return parser
}

func rootParserFixture(t *testing.T) Parser {
	t.Helper()
	parser, err := New(Definition{
		Name:      "checker",
		Version:   true,
		Options:   []Option{{Name: "profile", Short: "p", Placeholder: "FILE", Summary: "Configuration profile", Kind: ValueString}},
		Commands:  []Command{{Name: "docs", Summary: "Check documentation"}},
		Arguments: Arguments{Name: "PATH", Minimum: 1, Maximum: 2},
	})
	if err != nil {
		t.Fatal(err)
	}
	return parser
}

func validDefinition() Definition {
	return Definition{
		Name: "secverify", Summary: "Verify repositories", Version: true,
		Options: []Option{
			{Name: "root", Short: "r", Placeholder: "PATH", Summary: "Repository root", Kind: ValueString, Default: "."},
			{Name: "quiet", Short: "q", Summary: "Suppress progress", Kind: ValueBool, Default: "false"},
		},
		Commands: []Command{
			{Name: "static", Summary: "Run static verification"},
			{
				Name: "inspect", Summary: "Inspect one or two files", Arguments: Arguments{Name: "FILE", Minimum: 1, Maximum: 2},
				Options: []Option{
					{Name: "format", Short: "f", Placeholder: "FORMAT", Summary: "Output format", Kind: ValueString, Required: true},
					{Name: "strict", Short: "s", Summary: "Require strict input", Kind: ValueBool, Default: "false"},
				},
			},
		},
	}
}
