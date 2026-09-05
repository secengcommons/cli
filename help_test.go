package cli

import (
	"errors"
	"strconv"
	"strings"
	"testing"
	"unicode/utf8"
)

func FuzzQuotedLength(fuzz *testing.F) {
	for _, seed := range []string{"", "value", "a\"b\\c", "caf\u00e9", "\U0001f600"} {
		fuzz.Add(seed)
	}
	fuzz.Fuzz(func(t *testing.T, value string) {
		if len(value) > MaxInputBytes {
			return
		}
		wantValid := utf8.ValidString(value)
		if wantValid {
			for _, character := range value {
				if !strconv.IsPrint(character) {
					wantValid = false
					break
				}
			}
		}
		if got := validText(value); got != wantValid {
			t.Fatalf("validText(%q) = %t, want %t", value, got, wantValid)
		}
		if !wantValid {
			return
		}
		quoted := strconv.AppendQuote(nil, value)
		if got := quotedLength(value); got != len(quoted) {
			t.Fatalf("quoted length = %d, want %d", got, len(quoted))
		}
	})
}

func TestRootHelpIsDeterministic(t *testing.T) {
	parser := parserFixture(t)
	want := "Verify repositories\n\n" +
		"Usage:\n  secverify [flags] [command]\n\n" +
		"Available Commands:\n" +
		"  static   Run static verification\n" +
		"  inspect  Inspect one or two files\n" +
		"  help     Show help for a command\n" +
		"  version  Show version information\n\n" +
		"Flags:\n" +
		"  -r, --root PATH  Repository root (default \".\")\n" +
		"  -q, --quiet      Suppress progress\n" +
		"  -h, --help       Show help for secverify\n" +
		"  -v, --version    Show version information\n\n" +
		"Use \"secverify <command> --help\" for more information about a command\n"
	first, err := parser.Help("")
	if err != nil || string(first) != want {
		t.Fatalf("root help = (%q, %v)", first, err)
	}
	first[0] = 'X'
	second, err := parser.Help("")
	if err != nil || string(second) != want {
		t.Fatalf("repeated root help = (%q, %v)", second, err)
	}
}

func TestRootHelpOmitsAbsentSummary(t *testing.T) {
	t.Parallel()
	definition := validDefinition()
	definition.Summary = ""
	parser, err := New(definition)
	if err != nil {
		t.Fatal(err)
	}
	want := "Usage:\n  secverify [flags] [command]\n\n" +
		"Available Commands:\n" +
		"  static   Run static verification\n" +
		"  inspect  Inspect one or two files\n" +
		"  help     Show help for a command\n" +
		"  version  Show version information\n\n" +
		"Flags:\n" +
		"  -r, --root PATH  Repository root (default \".\")\n" +
		"  -q, --quiet      Suppress progress\n" +
		"  -h, --help       Show help for secverify\n" +
		"  -v, --version    Show version information\n\n" +
		"Use \"secverify <command> --help\" for more information about a command\n"
	value, err := parser.Help("")
	if err != nil || string(value) != want {
		t.Fatalf("root help = (%q, %v)", value, err)
	}
}

func TestCommandHelpIsDeterministic(t *testing.T) {
	parser := parserFixture(t)
	want := "Usage:\n  secverify inspect [flags] FILE...\n\n" +
		"Flags:\n" +
		"  -f, --format FORMAT  Output format (required)\n" +
		"  -s, --strict         Require strict input\n" +
		"  -h, --help           Show help for inspect\n\n" +
		"Global Flags:\n" +
		"  -r, --root PATH  Repository root (default \".\")\n" +
		"  -q, --quiet      Suppress progress\n" +
		"  -v, --version    Show version information\n"
	value, err := parser.Help("inspect")
	if err != nil || string(value) != want {
		t.Fatalf("command help = (%q, %v)", value, err)
	}
	if _, err = parser.Help("unknown"); !errors.Is(err, ErrInvocation) {
		t.Fatalf("unknown help error = %v", err)
	}
}

func TestCommandHelpWithoutGlobalOptions(t *testing.T) {
	parser, err := New(Definition{Name: "value", Summary: "Value", Commands: []Command{{Name: "run", Summary: "Run value"}}})
	if err != nil {
		t.Fatal(err)
	}
	want := "Usage:\n  value run [flags]\n\nFlags:\n  -h, --help  Show help for run\n"
	value, err := parser.Help("run")
	if err != nil || string(value) != want {
		t.Fatalf("minimal command help = (%q, %v)", value, err)
	}
}

func TestBuiltinCommandHelpIsDeterministic(t *testing.T) {
	parser := parserFixture(t)
	global := "\nGlobal Flags:\n" +
		"  -r, --root PATH  Repository root (default \".\")\n" +
		"  -q, --quiet      Suppress progress\n" +
		"  -v, --version    Show version information\n"
	for command, want := range map[string]string{
		"help": "Usage:\n  secverify help [flags] [COMMAND]\n\n" +
			"Flags:\n  -h, --help  Show help for help\n" + global,
		"version": "Usage:\n  secverify version [flags]\n\n" +
			"Flags:\n  -h, --help  Show help for version\n" + global,
	} {
		value, err := parser.Help(command)
		if err != nil || string(value) != want {
			t.Fatalf("builtin help %q = (%q, %v)", command, value, err)
		}
	}
	value, err := parser.Help("v")
	version, versionErr := parser.Help("version")
	if err != nil || versionErr != nil || string(value) != string(version) {
		t.Fatalf("version alias help = (%q, %v)", value, err)
	}
}

func TestRootActionHelpIsDeterministic(t *testing.T) {
	t.Parallel()
	parser, err := New(Definition{
		Name: "checker", Arguments: Arguments{Name: "PATH", Minimum: 1, Maximum: 2},
		Commands: []Command{{Name: "docs", Summary: "Check documentation"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	want := "Usage:\n  checker [flags] PATH...\n  checker [flags] [command]\n\n" +
		"Available Commands:\n" +
		"  docs  Check documentation\n" +
		"  help  Show help for a command\n\n" +
		"Flags:\n" +
		"  -h, --help  Show help for checker\n\n" +
		"Use \"checker <command> --help\" for more information about a command\n"
	value, err := parser.Help("")
	if err != nil || string(value) != want {
		t.Fatalf("root action help = (%q, %v)", value, err)
	}
}

func TestArgumentHelpShapes(t *testing.T) {
	t.Parallel()
	for arguments, want := range map[Arguments]string{
		{}:                                    "",
		{Name: "ARG", Maximum: 1}:             "[ARG]",
		{Name: "ARG", Minimum: 1, Maximum: 1}: "ARG",
		{Name: "ARG", Maximum: 2}:             "[ARG...]",
		{Name: "ARG", Minimum: 1, Maximum: 2}: "ARG...",
	} {
		output := helpOutput{value: make([]byte, 0)}
		writeArgumentHelp(&output, arguments)
		if got := string(output.value); got != want {
			t.Fatalf("argumentHelp(%#v) = %q", arguments, got)
		}
	}
}

func TestLongOptionLabel(t *testing.T) {
	t.Parallel()
	option := Option{Name: "long", Placeholder: "VALUE", Summary: "Value", Kind: ValueString}
	output := helpOutput{value: make([]byte, 0)}
	writeOptionLine(&output, option, optionLabelLength(option))
	if got := string(output.value); got != "      --long VALUE  Value\n" {
		t.Fatalf("option line = %q", got)
	}
}

func TestBooleanDefaultHelp(t *testing.T) {
	t.Parallel()
	option := Option{Name: "enabled", Summary: "Enable value", Kind: ValueBool, Default: "true"}
	output := helpOutput{value: make([]byte, 0)}
	writeOptionLine(&output, option, optionLabelLength(option))
	if got := strings.TrimSpace(string(output.value)); got != "--enabled  Enable value (default true)" {
		t.Fatalf("option summary = %q", got)
	}
	option = Option{Name: "format", Summary: "Output format", Kind: ValueString, Default: "false"}
	output = helpOutput{value: make([]byte, 0)}
	writeOptionLine(&output, option, optionLabelLength(option))
	if got := strings.TrimSpace(string(output.value)); got != "--format  Output format (default \"false\")" {
		t.Fatalf("string option summary = %q", got)
	}
}

func TestHelpBoundsCommandSelection(t *testing.T) {
	t.Parallel()
	parser := parserFixture(t)
	for _, command := range []string{strings.Repeat("x", MaxNameBytes+1), "line\nfeed", "direction\u202e"} {
		_, err := parser.Help(command)
		var detailed parseError
		if !errors.Is(err, ErrInvocation) || errors.As(err, &detailed) {
			t.Fatalf("Help(%q) error = %v", command, err)
		}
	}
}

func TestDiagnosticIsCobraStyle(t *testing.T) {
	parser := parserFixture(t)
	_, parseErr := parser.Parse([]string{"unknown"})
	value, err := parser.Diagnostic(parseErr)
	want := "Error: unknown command \"unknown\" for \"secverify\"\n\nRun 'secverify --help' for usage\n"
	if err != nil || string(value) != want {
		t.Fatalf("diagnostic = (%q, %v)", value, err)
	}
	value, err = parser.Diagnostic(ErrInvocation)
	if err != nil || string(value) != "Error: invalid command invocation\n\nRun 'secverify --help' for usage\n" {
		t.Fatalf("bounded diagnostic = (%q, %v)", value, err)
	}
	if _, err = parser.Diagnostic(errors.New("application")); !errors.Is(err, ErrInvocation) {
		t.Fatalf("application error = %v", err)
	}
	if _, err = parser.Diagnostic(errors.Join(ErrInvocation, errors.New("application"))); !errors.Is(err, ErrInvocation) {
		t.Fatalf("wrapped application error = %v", err)
	}
	if _, err = parser.Diagnostic(errors.Join(parseErr, errors.New("application"))); !errors.Is(err, ErrInvocation) {
		t.Fatalf("wrapped parser error = %v", err)
	}
	if _, err = parser.Diagnostic(forgedInvocationError{}); !errors.Is(err, ErrInvocation) {
		t.Fatalf("forged application error = %v", err)
	}
	var missing Parser
	if _, err = missing.Diagnostic(parseErr); !errors.Is(err, ErrDefinition) {
		t.Fatalf("nil parser error = %v", err)
	}
}

type forgedInvocationError struct{}

func (forgedInvocationError) Error() string { return ErrInvocation.Error() }

func (forgedInvocationError) Is(target error) bool { return errors.Is(target, ErrInvocation) }
