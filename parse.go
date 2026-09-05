package cli

import (
	"fmt"
	"slices"
	"strconv"
	"strings"
)

type parseError struct {
	message string
}

func (value parseError) Error() string { return value.message }

func (value parseError) Unwrap() error { return ErrInvocation }

type builtinState struct {
	helpSet    bool
	help       bool
	versionSet bool
	version    bool
}

func (parser Parser) Parse(arguments []string) (Invocation, error) {
	if !parser.available() {
		return Invocation{}, ErrDefinition
	}
	if !validInput(arguments) {
		return Invocation{}, ErrInvocation
	}
	var storage [maxInvocationOptions]optionValue
	root := storage[:len(parser.definition.options)]
	builtins := builtinState{}
	remaining, rootArguments, err := parser.parseOptions(arguments, nil, root, &builtins)
	if err != nil {
		return Invocation{}, err
	}
	return parser.resolveRootInvocation(root, &builtins, remaining, rootArguments)
}

func (parser Parser) resolveRootInvocation(
	root []optionValue,
	builtins *builtinState,
	remaining []string,
	rootArguments bool,
) (Invocation, error) {
	if builtins.help {
		return parser.helpInvocation(remaining)
	}
	if builtins.version {
		return versionInvocation(remaining)
	}
	if rootArguments && parser.definition.arguments.Maximum != 0 {
		return parser.rootInvocation(root, remaining)
	}
	if len(remaining) == 0 {
		return Invocation{action: ActionHelp}, nil
	}
	if remaining[0] == helpName {
		return parser.parseBuiltinInvocation(helpName, root, builtins, remaining[1:])
	}
	if (remaining[0] == versionName || remaining[0] == versionAlias) && parser.definition.version {
		return parser.parseBuiltinInvocation(versionName, root, builtins, remaining[1:])
	}
	if command, found := parser.definition.command(remaining[0]); found {
		return parser.parseCommandInvocation(command, root, builtins, remaining[1:])
	}
	if parser.definition.arguments.Maximum != 0 {
		return parser.rootInvocation(root, remaining)
	}
	return Invocation{}, invocationError("unknown command %q for %q", remaining[0], parser.definition.name)
}

func (parser Parser) parseBuiltinInvocation(
	name string,
	root []optionValue,
	builtins *builtinState,
	arguments []string,
) (Invocation, error) {
	remaining, _, err := parser.parseOptions(arguments, nil, root, builtins)
	if err != nil {
		return Invocation{}, err
	}
	if builtins.help {
		return parser.commandHelpInvocation(name, remaining)
	}
	if builtins.version {
		return versionInvocation(remaining)
	}
	if name == helpName {
		return parser.helpInvocation(remaining)
	}
	return versionInvocation(remaining)
}

func (parser Parser) parseCommandInvocation(
	command Command,
	root []optionValue,
	builtins *builtinState,
	arguments []string,
) (Invocation, error) {
	values := root[:len(root)+len(command.Options)]
	remaining, _, err := parser.parseOptions(arguments, command.Options, values, builtins)
	if err != nil {
		return Invocation{}, err
	}
	if builtins.help {
		return parser.commandHelpInvocation(command.Name, remaining)
	}
	if builtins.version {
		return versionInvocation(remaining)
	}
	if err = validateRequiredOptions(command.Options, values[len(root):]); err != nil {
		return Invocation{}, err
	}
	if err = validateArgumentCount(command.Name, command.Arguments, len(remaining)); err != nil {
		return Invocation{}, err
	}
	result := Invocation{
		rootOptions: parser.definition.options, localOptions: command.Options, action: ActionRun, command: command.Name,
	}
	retainArguments(&result, remaining)
	retainOverrides(&result, values)
	return result, nil
}

func (parser Parser) rootInvocation(root []optionValue, arguments []string) (Invocation, error) {
	if err := validateArgumentCount(parser.definition.name, parser.definition.arguments, len(arguments)); err != nil {
		return Invocation{}, err
	}
	result := Invocation{rootOptions: parser.definition.options, action: ActionRun}
	retainArguments(&result, arguments)
	retainOverrides(&result, root)
	return result, nil
}

func (parser Parser) parseOptions(
	arguments []string,
	local []Option,
	values []optionValue,
	builtins *builtinState,
) ([]string, bool, error) {
	for index := 0; index < len(arguments); index++ {
		argument := arguments[index]
		if argument == "--" {
			return arguments[index+1:], true, nil
		}
		name, inline, hasValue, option, err := splitOption(argument)
		if err != nil {
			return nil, false, err
		}
		if !option {
			return arguments[index:], false, nil
		}
		if handled, builtinErr := parser.setBuiltin(name, inline, hasValue, builtins); handled {
			if builtinErr != nil {
				return nil, false, builtinErr
			}
			continue
		}
		definition, valueIndex, found := findOption(name, parser.definition.options, local, true)
		if !found {
			return nil, false, invocationError("flag provided but not defined: -%s", name)
		}
		if definition.Kind == ValueString && !hasValue {
			index++
			if index == len(arguments) {
				return nil, false, invocationError("option --%s requires a value", definition.Name)
			}
			inline, hasValue = arguments[index], true
		}
		if err = setOption(definition, &values[valueIndex], inline, hasValue); err != nil {
			return nil, false, err
		}
	}
	return nil, false, nil
}

func splitOption(argument string) (name, value string, hasValue, option bool, err error) {
	if len(argument) < 2 || argument[0] != '-' {
		return "", "", false, false, nil
	}
	start := 1
	if argument[1] == '-' {
		start = 2
	}
	if start == len(argument) || argument[start] == '-' || argument[start] == '=' {
		return "", "", false, false, invocationError("bad flag syntax")
	}
	name, value, hasValue = strings.Cut(argument[start:], "=")
	return name, value, hasValue, true, nil
}

func (parser Parser) setBuiltin(name, value string, hasValue bool, state *builtinState) (bool, error) {
	switch name {
	case helpName, helpAlias:
		return true, setBoolean(helpName, value, hasValue, &state.help, &state.helpSet)
	case versionName, versionAlias:
		if parser.definition.version {
			return true, setBoolean(versionName, value, hasValue, &state.version, &state.versionSet)
		}
	}
	return false, nil
}

func setBoolean(name, value string, hasValue bool, destination, set *bool) error {
	if *set {
		return invocationError("option --%s provided more than once", name)
	}
	parsed := true
	var err error
	if hasValue {
		parsed, err = strconv.ParseBool(value)
		if err != nil {
			return invocationError("invalid value for --%s", name)
		}
	}
	*destination, *set = parsed, true
	return nil
}

func findOption(name string, root, local []Option, shortAliases bool) (Option, int, bool) {
	for index, option := range root {
		if name == option.Name || shortAliases && option.Short != "" && name == option.Short {
			return option, index, true
		}
	}
	for index, option := range local {
		if name == option.Name || shortAliases && option.Short != "" && name == option.Short {
			return option, len(root) + index, true
		}
	}
	return Option{}, 0, false
}

func setOption(definition Option, state *optionValue, value string, hasValue bool) error {
	if definition.Kind == ValueBool {
		return setBoolean(definition.Name, value, hasValue, &state.boolean, &state.set)
	}
	if state.set {
		return invocationError("option --%s provided more than once", definition.Name)
	}
	if !hasValue || value == "" {
		return invocationError("invalid value for --%s", definition.Name)
	}
	state.text = value
	state.set = true
	return nil
}

func retainOverrides(invocation *Invocation, values []optionValue) {
	count := 0
	for _, value := range values {
		if value.set {
			count++
		}
	}
	invocation.overrideCount = count
	if count == 0 {
		return
	}
	if count <= len(invocation.inlineOverrides) {
		result := invocation.inlineOverrides[:0]
		for index, value := range values {
			if value.set {
				result = append(result, optionOverride{text: value.text, index: uint16(index), boolean: value.boolean})
			}
		}
		return
	}
	invocation.overrides = make([]optionOverride, 0, count)
	for index, value := range values {
		if value.set {
			invocation.overrides = append(invocation.overrides, optionOverride{text: value.text, index: uint16(index), boolean: value.boolean})
		}
	}
}

func retainArguments(invocation *Invocation, arguments []string) {
	invocation.argumentCount = len(arguments)
	if len(arguments) <= len(invocation.inlineArguments) {
		copy(invocation.inlineArguments[:], arguments)
		return
	}
	invocation.arguments = slices.Clone(arguments)
}

func validateRequiredOptions(options []Option, values []optionValue) error {
	for index, option := range options {
		if option.Required && !values[index].set {
			return invocationError("required option --%s is missing", option.Name)
		}
	}
	return nil
}

func validateArgumentCount(command string, arguments Arguments, received int) error {
	if received >= arguments.Minimum && received <= arguments.Maximum {
		return nil
	}
	return argumentCountError(command, arguments, received)
}

func argumentCountError(command string, arguments Arguments, received int) error {
	switch {
	case arguments.Maximum == 0:
		return invocationError("command %q accepts no arguments", command)
	case arguments.Minimum == arguments.Maximum:
		return invocationError("command %q requires %d %s but received %d", command, arguments.Minimum, argumentWord(arguments.Minimum), received)
	case arguments.Minimum == 0:
		return invocationError("command %q accepts at most %d %s but received %d", command, arguments.Maximum, argumentWord(arguments.Maximum), received)
	default:
		return invocationError("command %q accepts %d to %d arguments but received %d", command, arguments.Minimum, arguments.Maximum, received)
	}
}

func argumentWord(count int) string {
	if count == 1 {
		return "argument"
	}
	return "arguments"
}

func (parser Parser) helpInvocation(arguments []string) (Invocation, error) {
	if len(arguments) == 0 {
		return Invocation{action: ActionHelp}, nil
	}
	if len(arguments) != 1 {
		return Invocation{}, invocationError("help accepts at most one command")
	}
	return parser.commandHelpInvocation(arguments[0], nil)
}

func (parser Parser) commandHelpInvocation(command string, arguments []string) (Invocation, error) {
	if len(arguments) != 0 {
		return Invocation{}, invocationError("help for %q accepts no arguments", command)
	}
	selected, found := parser.helpSubject(command)
	if !found {
		return Invocation{}, invocationError("unknown command %q for %q", command, parser.definition.name)
	}
	return Invocation{action: ActionHelp, command: selected.Name}, nil
}

func versionInvocation(arguments []string) (Invocation, error) {
	if len(arguments) != 0 {
		return Invocation{}, invocationError("version accepts no arguments")
	}
	return Invocation{action: ActionVersion}, nil
}

func (definition *admittedDefinition) command(name string) (Command, bool) {
	for _, command := range definition.commands {
		if command.Name == name {
			return command, true
		}
	}
	return Command{}, false
}

func validInput(arguments []string) bool {
	if len(arguments) > MaxInputArguments {
		return false
	}
	bytes := 0
	for _, argument := range arguments {
		if len(argument) > MaxInputBytes-bytes || !validText(argument) {
			return false
		}
		bytes += len(argument)
	}
	return true
}

func invocationError(format string, arguments ...any) error {
	return parseError{message: fmt.Sprintf(format, arguments...)}
}
