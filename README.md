<div align="center">
  <h1>Security Engineering Commons CLI</h1>
  Bounded command parsing and deterministic help for Go
</div>
<br>

CLI defines explicit commands and optional root actions with standard Go flag forms and [Cobra-style](https://github.com/spf13/cobra) help without reflection, templates or runtime dependencies

## Summary
- [Install](#install)
- [Use](#use)
- [Grammar](#grammar)
- [Help](#help)
- [Bounds](#bounds)
- [Performance](#performance)
  - [Benchmark method](#benchmark-method)
  - [Benchmark results](#benchmark-results)
- [Boundary](#boundary)
- [Verification](#verification)

## Install
```sh
go get github.com/secengcommons/cli
```

Requires Go 1.26 or newer

## Use
```go
parser, err := cli.New(cli.Definition{
    Name:    "secverify",
    Version: true,
    Options: []cli.Option{{
        Name: "root", Short: "r", Placeholder: "PATH",
        Summary: "Repository root", Kind: cli.ValueString, Default: ".",
    }},
    Commands: []cli.Command{
        {Name: "static", Summary: "Run static verification"},
        {Name: "test", Summary: "Run coverage and race verification"},
    },
})
if err != nil {
    return err
}

invocation, err := parser.Parse(arguments)
```

`New` validates and copies the complete definition. The application summary is optional. A parser is immutable and safe for concurrent use; every parse owns independent value state

`Invocation.Arguments` returns an owned copy. `ArgumentCount` and `Argument` provide allocation-free indexed access to the same immutable arguments

Applications with a root action set `Definition.Arguments`. A root action returns `ActionRun` with an empty command name

## Grammar
The parser accepts:
```text
APPLICATION [GLOBAL OPTIONS] [COMMAND [GLOBAL OR COMMAND OPTIONS] [ARGUMENTS]]
APPLICATION [GLOBAL OPTIONS] ROOT_ARGUMENTS
APPLICATION help [COMMAND]
APPLICATION -h
APPLICATION --help
APPLICATION -v
APPLICATION --v
APPLICATION --version
APPLICATION v
APPLICATION version
```

String options accept single-dash and double-dash names, equals values and separate values:
```text
-option value
--option value
-option=value
--option=value
```

Boolean options accept an omitted value as `true` or an equals value accepted by [`strconv.ParseBool`](https://pkg.go.dev/strconv#ParseBool). They do not consume a separate following token

Global options may appear before or after the command. Options are never repeatable. String options reject present empty values. Root options cannot be required

`--` ends option parsing. After a command it makes every remaining token positional. Where a root action exists, a root-level `--` selects that action and makes every remaining token a root argument

A positional token which exactly names a command selects that command. Where a root action exists, any other positional token begins it. An empty invocation returns root help

Where no root action exists, unknown commands fail before application work begins. Unknown options, duplicate options, missing values, unexpected arguments and absent required options fail at the same boundary

## Help

Root help contains the optional application summary, usage, commands and global options. Command help begins with usage then lists local and inherited global options. Command descriptions appear only in the root command list. Definition order controls display order

Help is deterministic UTF-8 text with one final line feed. It does not inspect the terminal, use colour, load configuration or read environment variables

`Diagnostic` renders direct parser invocation errors as `Error: ...` followed by the root-help command. It rejects wrapped and unrelated application errors

## Bounds

Limit | Value
--- | ---:
Commands | 64
Options per owner | 64
Input arguments | 256
Command positional arguments | 255
Root positional arguments | 255
Input bytes | 64 KiB
Name bytes | 64
Summary bytes | 256
Definition text | 1 MiB

Counts and bytes are checked before application work. Names use lowercase alphanumeric segments separated by single hyphens; placeholders use the uppercase equivalent. Summaries cannot contain surrounding whitespace. Invalid UTF-8, control characters and ambiguous aliases are rejected

## Performance

Benchmarks cover every exported operation and report allocations

### Benchmark method

(5 September 2026) - The measurements use:
- Linux AMD64
- 13th Gen Intel Core i5-13400F
- Go 1.26.6
- five 500 ms samples per operation
- the median of each five-sample set

### Benchmark results

Operation | Time | Bytes | Allocations
--- | ---: | ---: | ---:
Parse one command with a global string option | 191.0 ns | 0 | 0
Parse one command with two options and two arguments | 288.8 ns | 0 | 0
Parse a two-argument root action | 143.8 ns | 0 | 0
Construct the representative parser | 897.2 ns | 560 | 2
Construct a root-only parser | 90.65 ns | 0 | 0
Render root help | 652.9 ns | 480 | 1
Render command help | 661.4 ns | 352 | 1
Render a diagnostic | 71.32 ns | 96 | 1

The caller-owned byte slice accounts for each help and diagnostic allocation. Common parse paths remain allocation-free. Run the complete set with:
```sh
go test -run '^$' -bench . -benchmem -benchtime=500ms -count=5 ./...
```

## Boundary

Definitions come from the application. Arguments are untrusted input

CLI parses arguments and renders help and diagnostics. The application owns command authorisation, execution, context, output and exit status. Command-line arguments remain visible to the operating system

## Verification
Run the complete local gate with:
```sh
go -C tools tool secverify --root .. all
```
