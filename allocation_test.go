package cli

import (
	"errors"
	"testing"
)

var allocatedInvocation Invocation
var allocatedHelp []byte
var allocatedParser Parser
var allocatedArguments []string
var allocatedArgument string
var allocatedArgumentFound bool

func TestParseAllocationBounds(t *testing.T) {
	parser := parserFixture(t)
	var staticErr error
	static := testing.AllocsPerRun(1_000, func() {
		allocatedInvocation, staticErr = parser.Parse([]string{"static", "--root", "repository"})
	})
	var commandErr error
	command := testing.AllocsPerRun(1_000, func() {
		allocatedInvocation, commandErr = parser.Parse([]string{"inspect", "--format", "json", "first", "second"})
	})
	if staticErr != nil || commandErr != nil || static != 0 || command != 0 {
		t.Fatalf("parse allocations = (static %.0f, command %.0f, errors %v and %v)", static, command, staticErr, commandErr)
	}
}

func TestResultAllocationBounds(t *testing.T) {
	parser := parserFixture(t)
	invocation, err := parser.Parse([]string{"inspect", "--format", "json", "first", "second"})
	if err != nil {
		t.Fatal(err)
	}
	var rootHelpErr error
	rootHelp := testing.AllocsPerRun(1_000, func() {
		allocatedHelp, rootHelpErr = parser.Help("")
	})
	var commandHelpErr error
	commandHelp := testing.AllocsPerRun(1_000, func() {
		allocatedHelp, commandHelpErr = parser.Help("inspect")
	})
	_, diagnosticErr := parser.Parse([]string{"unknown"})
	var renderedDiagnosticErr error
	diagnostic := testing.AllocsPerRun(1_000, func() {
		allocatedHelp, renderedDiagnosticErr = parser.Diagnostic(diagnosticErr)
	})
	arguments := testing.AllocsPerRun(1_000, func() {
		allocatedArguments = invocation.Arguments()
	})
	argument := testing.AllocsPerRun(1_000, func() {
		allocatedArgument, allocatedArgumentFound = invocation.Argument(1)
	})
	if rootHelpErr != nil || commandHelpErr != nil || renderedDiagnosticErr != nil {
		t.Fatalf("errors = (%v, %v, %v)", rootHelpErr, commandHelpErr, renderedDiagnosticErr)
	}
	if rootHelp != 1 || commandHelp != 1 || diagnostic != 1 || arguments != 1 || argument != 0 {
		t.Fatalf("result allocations = root help %.0f, command help %.0f, diagnostic %.0f, arguments %.0f, argument %.0f",
			rootHelp, commandHelp, diagnostic, arguments, argument)
	}
	if !allocatedArgumentFound || allocatedArgument != "second" {
		t.Fatalf("indexed argument = (%q, %t)", allocatedArgument, allocatedArgumentFound)
	}
}

func TestSimpleActionsAreAllocationFree(t *testing.T) {
	parser := parserFixture(t)
	for _, arguments := range [][]string{
		{"static"}, nil, {"help"}, {"version"}, {"-h"}, {"-v"},
		{"static", "--help"}, {"static", "--version"}, {"help", "static"}, {"v", "--help"},
	} {
		var parseErr error
		allocations := testing.AllocsPerRun(1_000, func() {
			allocatedInvocation, parseErr = parser.Parse(arguments)
		})
		if parseErr != nil || allocations != 0 {
			t.Fatalf("Parse(%q) = (%.0f allocations, %v)", arguments, allocations, parseErr)
		}
	}
}

func TestRootActionAllocationBound(t *testing.T) {
	parser, err := New(Definition{Name: "check", Arguments: Arguments{Name: "PATH", Minimum: 1, Maximum: 1}})
	if err != nil {
		t.Fatal(err)
	}
	var parseErr error
	allocations := testing.AllocsPerRun(1_000, func() {
		allocatedInvocation, parseErr = parser.Parse([]string{"README.md"})
	})
	if parseErr != nil || allocations != 0 {
		t.Fatalf("root action = (%.0f allocations, %v)", allocations, parseErr)
	}
}

func TestSpillAllocationBounds(t *testing.T) {
	parser, err := New(Definition{
		Name: "check",
		Options: []Option{
			{Name: "alpha", Summary: "Select alpha", Kind: ValueBool},
			{Name: "bravo", Summary: "Select bravo", Kind: ValueBool},
			{Name: "charlie", Summary: "Select charlie", Kind: ValueBool},
		},
		Arguments: Arguments{Name: "PATH", Minimum: 1, Maximum: 3},
	})
	if err != nil {
		t.Fatal(err)
	}
	var parseErr error
	overrides := testing.AllocsPerRun(1_000, func() {
		allocatedInvocation, parseErr = parser.Parse([]string{"--alpha", "--bravo", "--charlie", "one"})
	})
	if parseErr != nil {
		t.Fatal(parseErr)
	}
	arguments := testing.AllocsPerRun(1_000, func() {
		allocatedInvocation, parseErr = parser.Parse([]string{"one", "two", "three"})
	})
	if parseErr != nil {
		t.Fatal(parseErr)
	}
	both := testing.AllocsPerRun(1_000, func() {
		allocatedInvocation, parseErr = parser.Parse([]string{"--alpha", "--bravo", "--charlie", "one", "two", "three"})
	})
	if parseErr != nil || overrides != 1 || arguments != 1 || both != 2 {
		t.Fatalf("spill allocations = overrides %.0f, arguments %.0f, both %.0f, error %v", overrides, arguments, both, parseErr)
	}
}

func TestHelpAllocationBoundAcrossShapes(t *testing.T) {
	parser, err := New(Definition{
		Name: "checker", Summary: "Check material",
		Options: []Option{{
			Name: "profile", Placeholder: "FILE", Summary: "Configuration profile", Kind: ValueString, Default: "a\"b\\c",
		}},
		Commands: []Command{{
			Name: "docs", Summary: "Check documentation",
			Options: []Option{{Name: "strict", Summary: "Require strict input", Kind: ValueBool, Default: "true"}},
		}},
		Arguments: Arguments{Name: "PATH", Minimum: 1, Maximum: 8}, Version: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, command := range []string{"", "docs", "help", "version"} {
		var helpErr error
		allocations := testing.AllocsPerRun(1_000, func() {
			allocatedHelp, helpErr = parser.Help(command)
		})
		if helpErr != nil || allocations != 1 {
			t.Fatalf("Help(%q) = (%.0f allocations, %v)", command, allocations, helpErr)
		}
	}
}

func TestConstructorAllocationBounds(t *testing.T) {
	var constructionErr error
	definition := validDefinition()
	ordinary := testing.AllocsPerRun(1_000, func() {
		allocatedParser, constructionErr = New(definition)
	})
	if constructionErr != nil || ordinary != 2 {
		t.Fatalf("ordinary construction = (%.0f allocations, %v)", ordinary, constructionErr)
	}
	root := Definition{Name: "check", Arguments: Arguments{Name: "PATH", Minimum: 1, Maximum: 1}}
	rootOnly := testing.AllocsPerRun(1_000, func() {
		allocatedParser, constructionErr = New(root)
	})
	if constructionErr != nil || rootOnly != 0 {
		t.Fatalf("root-only construction = (%.0f allocations, %v)", rootOnly, constructionErr)
	}
}

func TestOversizedInputRejectsWithoutAllocation(t *testing.T) {
	parser := parserFixture(t)
	arguments := []string{string(make([]byte, MaxInputBytes+1))}
	var parseErr error
	allocations := testing.AllocsPerRun(1_000, func() {
		allocatedInvocation, parseErr = parser.Parse(arguments)
	})
	if !errors.Is(parseErr, ErrInvocation) || allocations != 0 {
		t.Fatalf("oversized input = (%.0f allocations, %v)", allocations, parseErr)
	}
}
