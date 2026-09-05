package cli

import (
	"strings"
	"testing"
)

var benchmarkParserResult Parser
var benchmarkInvocationResult Invocation
var benchmarkError error
var benchmarkAction Action
var benchmarkText string
var benchmarkBool bool
var benchmarkFound bool
var benchmarkCount int
var benchmarkBytes []byte
var benchmarkArguments []string

func BenchmarkParseStatic(benchmark *testing.B) {
	parser := benchmarkParser(benchmark)
	arguments := []string{"static", "--root", "repository"}
	benchmark.ReportAllocs()
	for benchmark.Loop() {
		benchmarkInvocationResult, benchmarkError = parser.Parse(arguments)
	}
	if benchmarkError != nil {
		benchmark.Fatal(benchmarkError)
	}
}

func BenchmarkParseCommand(benchmark *testing.B) {
	parser := benchmarkParser(benchmark)
	arguments := []string{"inspect", "--format", "json", "--strict", "first", "second"}
	benchmark.ReportAllocs()
	for benchmark.Loop() {
		benchmarkInvocationResult, benchmarkError = parser.Parse(arguments)
	}
	if benchmarkError != nil {
		benchmark.Fatal(benchmarkError)
	}
}

func BenchmarkParseDefault(benchmark *testing.B) {
	parser := benchmarkParser(benchmark)
	arguments := []string{"static"}
	benchmark.ReportAllocs()
	for benchmark.Loop() {
		benchmarkInvocationResult, benchmarkError = parser.Parse(arguments)
	}
	if benchmarkError != nil {
		benchmark.Fatal(benchmarkError)
	}
}

func BenchmarkParseRootAction(benchmark *testing.B) {
	parser, err := New(Definition{
		Name: "check", Arguments: Arguments{Name: "PATH", Minimum: 1, Maximum: 2},
	})
	if err != nil {
		benchmark.Fatal(err)
	}
	arguments := []string{"README.md", "docs/"}
	benchmark.ReportAllocs()
	for benchmark.Loop() {
		benchmarkInvocationResult, benchmarkError = parser.Parse(arguments)
	}
	if benchmarkError != nil {
		benchmark.Fatal(benchmarkError)
	}
}

func BenchmarkParseHelp(benchmark *testing.B) {
	parser := benchmarkParser(benchmark)
	arguments := []string{"static", "--help"}
	benchmark.ReportAllocs()
	for benchmark.Loop() {
		benchmarkInvocationResult, benchmarkError = parser.Parse(arguments)
	}
	if benchmarkError != nil {
		benchmark.Fatal(benchmarkError)
	}
}

func BenchmarkNew(benchmark *testing.B) {
	definition := validDefinition()
	benchmark.ReportAllocs()
	for benchmark.Loop() {
		benchmarkParserResult, benchmarkError = New(definition)
	}
	if benchmarkError != nil {
		benchmark.Fatal(benchmarkError)
	}
}

func BenchmarkNewRootAction(benchmark *testing.B) {
	definition := Definition{Name: "check", Arguments: Arguments{Name: "PATH", Minimum: 1, Maximum: 1}}
	benchmark.ReportAllocs()
	for benchmark.Loop() {
		benchmarkParserResult, benchmarkError = New(definition)
	}
	if benchmarkError != nil {
		benchmark.Fatal(benchmarkError)
	}
}

func BenchmarkRejectOversizedInput(benchmark *testing.B) {
	parser := benchmarkParser(benchmark)
	arguments := []string{strings.Repeat("x", 16<<20)}
	benchmark.ReportAllocs()
	for benchmark.Loop() {
		benchmarkInvocationResult, benchmarkError = parser.Parse(arguments)
	}
	if benchmarkError == nil {
		benchmark.Fatal("oversized input was accepted")
	}
}

func BenchmarkDiagnostic(benchmark *testing.B) {
	parser := benchmarkParser(benchmark)
	_, parseErr := parser.Parse([]string{"unknown"})
	benchmark.ReportAllocs()
	for benchmark.Loop() {
		benchmarkBytes, benchmarkError = parser.Diagnostic(parseErr)
	}
	if benchmarkError != nil {
		benchmark.Fatal(benchmarkError)
	}
}

func BenchmarkArguments(benchmark *testing.B) {
	invocation := benchmarkInvocation(benchmark)
	benchmark.ReportAllocs()
	for benchmark.Loop() {
		benchmarkArguments = invocation.Arguments()
	}
}

func BenchmarkArgument(benchmark *testing.B) {
	invocation := benchmarkInvocation(benchmark)
	benchmark.ReportAllocs()
	for benchmark.Loop() {
		benchmarkText, benchmarkFound = invocation.Argument(1)
	}
}

func BenchmarkArgumentCount(benchmark *testing.B) {
	invocation := benchmarkInvocation(benchmark)
	benchmark.ReportAllocs()
	for benchmark.Loop() {
		benchmarkCount = invocation.ArgumentCount()
	}
}

func BenchmarkAction(benchmark *testing.B) {
	invocation := benchmarkInvocation(benchmark)
	benchmark.ReportAllocs()
	for benchmark.Loop() {
		benchmarkAction = invocation.Action()
	}
}

func BenchmarkCommand(benchmark *testing.B) {
	invocation := benchmarkInvocation(benchmark)
	benchmark.ReportAllocs()
	for benchmark.Loop() {
		benchmarkText = invocation.Command()
	}
}

func BenchmarkIsSet(benchmark *testing.B) {
	invocation := benchmarkInvocation(benchmark)
	benchmark.ReportAllocs()
	for benchmark.Loop() {
		benchmarkBool = invocation.IsSet("format")
	}
}

func BenchmarkString(benchmark *testing.B) {
	invocation := benchmarkInvocation(benchmark)
	benchmark.ReportAllocs()
	for benchmark.Loop() {
		benchmarkText, benchmarkFound = invocation.String("format")
	}
}

func BenchmarkBool(benchmark *testing.B) {
	invocation := benchmarkInvocation(benchmark)
	benchmark.ReportAllocs()
	for benchmark.Loop() {
		benchmarkBool, benchmarkFound = invocation.Bool("strict")
	}
}

func BenchmarkRootHelp(benchmark *testing.B) {
	parser := benchmarkParser(benchmark)
	benchmark.ReportAllocs()
	for benchmark.Loop() {
		benchmarkBytes, benchmarkError = parser.Help("")
	}
	if benchmarkError != nil {
		benchmark.Fatal(benchmarkError)
	}
}

func BenchmarkCommandHelp(benchmark *testing.B) {
	parser := benchmarkParser(benchmark)
	benchmark.ReportAllocs()
	for benchmark.Loop() {
		benchmarkBytes, benchmarkError = parser.Help("inspect")
	}
	if benchmarkError != nil {
		benchmark.Fatal(benchmarkError)
	}
}

func benchmarkParser(benchmark *testing.B) Parser {
	benchmark.Helper()
	parser, err := New(validDefinition())
	if err != nil {
		benchmark.Fatal(err)
	}
	return parser
}

func benchmarkInvocation(benchmark *testing.B) Invocation {
	benchmark.Helper()
	parser := benchmarkParser(benchmark)
	invocation, err := parser.Parse([]string{"inspect", "--format", "json", "--strict", "first", "second"})
	if err != nil {
		benchmark.Fatal(err)
	}
	return invocation
}
