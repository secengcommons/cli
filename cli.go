package cli

import (
	"errors"
	"slices"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"
)

const MaxCommands = 64
const MaxOptions = 64
const MaxInputArguments = 256
const MaxPositionalArguments = MaxInputArguments - 1
const MaxInputBytes = 64 << 10
const MaxNameBytes = 64
const MaxSummaryBytes = 256
const MaxDefinitionBytes = 1 << 20
const maxInvocationOptions = MaxOptions * 2

// Two inline values cover the measured common invocation shape without heap allocation
const inlineInvocationValues = 2

const helpName = "help"
const helpAlias = "h"
const helpSummary = "Show help for a command"
const versionName = "version"
const versionAlias = "v"
const versionSummary = "Show version information"

var ErrDefinition = errors.New("invalid command definition")
var ErrInvocation = errors.New("invalid command invocation")

type ValueKind uint8

const (
	ValueString ValueKind = iota + 1
	ValueBool
)

type Definition struct {
	Name      string
	Summary   string
	Options   []Option
	Commands  []Command
	Arguments Arguments
	Version   bool
}

type Command struct {
	Name      string
	Summary   string
	Options   []Option
	Arguments Arguments
}

type Option struct {
	Name        string
	Short       string
	Placeholder string
	Summary     string
	Kind        ValueKind
	Default     string
	Required    bool
}

type Arguments struct {
	Name    string
	Minimum int
	Maximum int
}

type Action uint8

const (
	ActionRun Action = iota + 1
	ActionHelp
	ActionVersion
)

type Parser struct {
	definition admittedDefinition
}

type Invocation struct {
	rootOptions     []Option
	localOptions    []Option
	action          Action
	command         string
	argumentCount   int
	overrideCount   int
	inlineArguments [inlineInvocationValues]string
	inlineOverrides [inlineInvocationValues]optionOverride
	arguments       []string
	overrides       []optionOverride
}

type admittedDefinition struct {
	name      string
	summary   string
	options   []Option
	commands  []Command
	arguments Arguments
	version   bool
}

type optionValue struct {
	text    string
	boolean bool
	set     bool
}

type optionOverride struct {
	text    string
	index   uint16
	boolean bool
}

func (parser Parser) available() bool { return parser.definition.name != "" }

func New(definition Definition) (Parser, error) {
	admitted, err := admitDefinition(definition)
	if err != nil {
		return Parser{}, err
	}
	return Parser{definition: admitted}, nil
}

func (invocation Invocation) Action() Action { return invocation.action }

func (invocation Invocation) Command() string { return invocation.command }

func (invocation Invocation) Arguments() []string {
	if invocation.argumentCount == 0 {
		return nil
	}
	if invocation.argumentCount <= len(invocation.inlineArguments) {
		return slices.Clone(invocation.inlineArguments[:invocation.argumentCount])
	}
	return slices.Clone(invocation.arguments)
}

func (invocation Invocation) ArgumentCount() int { return invocation.argumentCount }

func (invocation Invocation) Argument(index int) (string, bool) {
	if index < 0 || index >= invocation.argumentCount {
		return "", false
	}
	if invocation.argumentCount <= len(invocation.inlineArguments) {
		return invocation.inlineArguments[index], true
	}
	return invocation.arguments[index], true
}

func (invocation Invocation) IsSet(name string) bool {
	_, index, found := invocation.option(name)
	if !found {
		return false
	}
	_, found = invocation.override(index)
	return found
}

func (invocation Invocation) String(name string) (string, bool) {
	option, index, found := invocation.option(name)
	if !found || option.Kind != ValueString {
		return "", false
	}
	if value, overridden := invocation.override(index); overridden {
		return value.text, true
	}
	return option.Default, true
}

func (invocation Invocation) Bool(name string) (bool, bool) {
	option, index, found := invocation.option(name)
	if !found || option.Kind != ValueBool {
		return false, false
	}
	if value, overridden := invocation.override(index); overridden {
		return value.boolean, true
	}
	return option.Default == "true", true
}

func (invocation Invocation) option(name string) (Option, int, bool) {
	return findOption(name, invocation.rootOptions, invocation.localOptions, false)
}

func (invocation Invocation) override(index int) (optionOverride, bool) {
	if invocation.overrideCount <= len(invocation.inlineOverrides) {
		for _, value := range invocation.inlineOverrides[:invocation.overrideCount] {
			if int(value.index) == index {
				return value, true
			}
		}
		return optionOverride{}, false
	}
	for _, value := range invocation.overrides {
		if int(value.index) == index {
			return value, true
		}
	}
	return optionOverride{}, false
}

func admitDefinition(definition Definition) (admittedDefinition, error) {
	if !validName(definition.Name) || !validOptionalSummary(definition.Summary) || !validRootArguments(definition.Arguments) ||
		!validDefinitionActions(definition) || !validDefinitionCounts(definition) || definitionBytes(definition) > MaxDefinitionBytes {
		return admittedDefinition{}, ErrDefinition
	}
	optionPool := make([]Option, definitionOptionCount(definition))
	options := optionPool[:len(definition.Options):len(definition.Options)]
	if err := admitOptions(options, definition.Options, true, definition.Version, nil); err != nil {
		return admittedDefinition{}, err
	}
	commands := make([]Command, len(definition.Commands))
	var names [MaxCommands]string
	optionOffset := len(options)
	for index, command := range definition.Commands {
		if !validCommand(command, definition.Version) || slices.Contains(names[:index], command.Name) {
			return admittedDefinition{}, ErrDefinition
		}
		end := optionOffset + len(command.Options)
		commandOptions := optionPool[optionOffset:end:end]
		if err := admitOptions(commandOptions, command.Options, false, definition.Version, options); err != nil {
			return admittedDefinition{}, err
		}
		command.Options = commandOptions
		optionOffset = end
		names[index] = command.Name
		commands[index] = command
	}
	return admittedDefinition{
		name: definition.Name, summary: definition.Summary, options: options, commands: commands,
		arguments: definition.Arguments, version: definition.Version,
	}, nil
}

func definitionOptionCount(definition Definition) int {
	count := len(definition.Options)
	for _, command := range definition.Commands {
		count += len(command.Options)
	}
	return count
}

func validDefinitionActions(definition Definition) bool {
	return len(definition.Commands) != 0 || definition.Arguments.Maximum != 0
}

func validDefinitionCounts(definition Definition) bool {
	if len(definition.Commands) > MaxCommands || len(definition.Options) > MaxOptions {
		return false
	}
	for _, command := range definition.Commands {
		if len(command.Options) > MaxOptions {
			return false
		}
	}
	return true
}

func definitionBytes(definition Definition) int {
	total := len(definition.Name) + len(definition.Summary) + len(definition.Arguments.Name)
	for _, option := range definition.Options {
		total += optionBytes(option)
	}
	for _, command := range definition.Commands {
		total += len(command.Name) + len(command.Summary) + len(command.Arguments.Name)
		for _, option := range command.Options {
			total += optionBytes(option)
		}
	}
	return total
}

func optionBytes(option Option) int {
	return len(option.Name) + len(option.Short) + len(option.Placeholder) + len(option.Summary) + len(option.Default)
}

func admitOptions(destination, options []Option, root, version bool, inherited []Option) error {
	copy(destination, options)
	for index, option := range destination {
		if !validOption(option, root, version) || optionConflict(option, destination[:index]) || optionConflict(option, inherited) {
			return ErrDefinition
		}
	}
	return nil
}

func validCommand(command Command, version bool) bool {
	reserved := command.Name == helpName || version && (command.Name == versionName || command.Name == versionAlias)
	return !reserved && validName(command.Name) && validSummary(command.Summary) && validArguments(command.Arguments)
}

func validOption(option Option, root, version bool) bool {
	if !validOptionIdentity(option, version) || !validOptionOwnership(option, root) {
		return false
	}
	return validOptionValue(option)
}

func validOptionIdentity(option Option, version bool) bool {
	reservedVersion := version && (option.Name == versionName || option.Name == versionAlias || option.Short == versionAlias)
	return validName(option.Name) && validShortName(option.Short) && validSummary(option.Summary) &&
		option.Name != helpName && option.Name != helpAlias && !reservedVersion && option.Short != helpAlias &&
		(option.Short == "" || option.Name != option.Short)
}

func validOptionOwnership(option Option, root bool) bool {
	return (!root || !option.Required) && (!option.Required || option.Default == "")
}

func validOptionValue(option Option) bool {
	switch option.Kind {
	case ValueString:
		return validPlaceholder(option.Placeholder) && validText(option.Default)
	case ValueBool:
		return option.Placeholder == "" && (option.Default == "" || option.Default == "false" || option.Default == "true")
	default:
		return false
	}
}

func validArguments(arguments Arguments) bool {
	if arguments.Minimum < 0 || arguments.Maximum < arguments.Minimum || arguments.Maximum > MaxPositionalArguments {
		return false
	}
	if arguments.Maximum == 0 {
		return arguments.Name == ""
	}
	return validPlaceholder(arguments.Name)
}

func validRootArguments(arguments Arguments) bool {
	return validArguments(arguments) && (arguments.Maximum == 0 || arguments.Minimum > 0)
}

func optionConflict(option Option, existing []Option) bool {
	for _, candidate := range existing {
		if option.Name == candidate.Name || option.Short != "" && option.Short == candidate.Short || option.Name == candidate.Short ||
			option.Short != "" && option.Short == candidate.Name {
			return true
		}
	}
	return false
}

func validName(value string) bool {
	return validHyphenated(value, 'a', 'z')
}

func validShortName(value string) bool {
	return value == "" || len(value) == 1 && asciiAlphaNumeric(value[0])
}

func asciiAlphaNumeric(value byte) bool {
	return value >= 'a' && value <= 'z' || value >= 'A' && value <= 'Z' || value >= '0' && value <= '9'
}

func validPlaceholder(value string) bool {
	return validHyphenated(value, 'A', 'Z')
}

func validHyphenated(value string, first, last byte) bool {
	if !validFirstCharacter(value, first, last) || value[len(value)-1] == '-' {
		return false
	}
	for index := 1; index < len(value); index++ {
		character := value[index]
		if character == '-' && value[index-1] == '-' {
			return false
		}
		if character != '-' && !asciiLetterOrDigit(character, first, last) {
			return false
		}
	}
	return true
}

func validFirstCharacter(value string, first, last byte) bool {
	return value != "" && len(value) <= MaxNameBytes && value[0] >= first && value[0] <= last
}

func asciiLetterOrDigit(value, first, last byte) bool {
	return value >= first && value <= last || value >= '0' && value <= '9'
}

func validSummary(value string) bool {
	return value != "" && len(value) <= MaxSummaryBytes && strings.TrimSpace(value) == value && validText(value)
}

func validOptionalSummary(value string) bool {
	return value == "" || validSummary(value)
}

func validText(value string) bool {
	for index := 0; index < len(value); index++ {
		character := value[index]
		if character >= utf8.RuneSelf {
			return validUnicodeText(value)
		}
		if character < ' ' || character == unicode.MaxASCII {
			return false
		}
	}
	return true
}

func validUnicodeText(value string) bool {
	if !utf8.ValidString(value) {
		return false
	}
	for _, character := range value {
		if !strconv.IsPrint(character) {
			return false
		}
	}
	return true
}
