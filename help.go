package cli

import (
	"strconv"
	"unicode/utf8"
)

type helpOutput struct {
	// A nil value counts bytes while a non-nil value emits them
	value []byte
	size  int
}

func (output *helpOutput) writeString(value string) {
	if output.value == nil {
		output.size += len(value)
		return
	}
	output.value = append(output.value, value...)
}

func (output *helpOutput) writeByte(value byte) {
	if output.value == nil {
		output.size++
		return
	}
	output.value = append(output.value, value)
}

func (output *helpOutput) writeQuoted(value string) {
	if output.value == nil {
		output.size += quotedLength(value)
		return
	}
	output.value = strconv.AppendQuote(output.value, value)
}

func (parser Parser) Help(command string) ([]byte, error) {
	if !parser.available() {
		return nil, ErrDefinition
	}
	if command == "" {
		return renderRootHelp(&parser.definition), nil
	}
	if !validName(command) {
		return nil, ErrInvocation
	}
	selected, found := parser.helpSubject(command)
	if !found {
		return nil, invocationError("unknown command %q for %q", command, parser.definition.name)
	}
	return renderCommandHelp(&parser.definition, selected), nil
}

func renderRootHelp(definition *admittedDefinition) []byte {
	var size helpOutput
	writeRootHelp(&size, definition)
	result := helpOutput{value: make([]byte, 0, size.size)}
	writeRootHelp(&result, definition)
	return result.value
}

func renderCommandHelp(definition *admittedDefinition, command Command) []byte {
	var size helpOutput
	writeCommandHelp(&size, definition, command)
	result := helpOutput{value: make([]byte, 0, size.size)}
	writeCommandHelp(&result, definition, command)
	return result.value
}

func (parser Parser) helpSubject(name string) (Command, bool) {
	if command, found := parser.definition.command(name); found {
		return command, true
	}
	switch name {
	case helpName:
		return Command{Name: helpName, Arguments: Arguments{Name: "COMMAND", Maximum: 1}}, true
	case versionName, versionAlias:
		if parser.definition.version {
			return Command{Name: versionName}, true
		}
	}
	return Command{}, false
}

func (parser Parser) Diagnostic(err error) ([]byte, error) {
	if !parser.available() {
		return nil, ErrDefinition
	}
	message, found := invocationMessage(err)
	if !found {
		return nil, ErrInvocation
	}
	result := make([]byte, 0, len("Error: \n\nRun ' --help' for usage\n")+len(message)+len(parser.definition.name))
	result = append(result, "Error: "...)
	result = append(result, message...)
	result = append(result, "\n\nRun '"...)
	result = append(result, parser.definition.name...)
	result = append(result, " --help' for usage\n"...)
	return result, nil
}

func invocationMessage(err error) (string, bool) {
	//nolint:errorlint // Exact identities reject wrapped and forged application errors
	switch value := err.(type) {
	case parseError:
		return value.Error(), true
	default:
		if err == ErrInvocation {
			return ErrInvocation.Error(), true
		}
		return "", false
	}
}

func writeRootHelp(output *helpOutput, definition *admittedDefinition) {
	if definition.summary != "" {
		output.writeString(definition.summary)
		output.writeString("\n\n")
	}
	output.writeString("Usage:\n")
	if definition.arguments.Maximum != 0 {
		writeUsagePrefix(output, definition.name)
		output.writeString("[flags] ")
		writeArgumentHelp(output, definition.arguments)
		output.writeByte('\n')
	}
	writeUsagePrefix(output, definition.name)
	output.writeString("[flags] [command]\n")
	output.writeString("\nAvailable Commands:\n")
	width := commandWidth(definition)
	for _, command := range definition.commands {
		writeHelpLine(output, command.Name, command.Summary, width)
	}
	writeHelpLine(output, helpName, helpSummary, width)
	if definition.version {
		writeHelpLine(output, versionName, versionSummary, width)
	}
	output.writeString("\nFlags:\n")
	writeOptionHelp(output, definition.options, definition.name, true, definition.version)
	output.writeString("\nUse \"")
	output.writeString(definition.name)
	output.writeString(" <command> --help\" for more information about a command\n")
}

func writeCommandHelp(output *helpOutput, definition *admittedDefinition, command Command) {
	output.writeString("Usage:\n")
	writeUsagePrefix(output, definition.name)
	output.writeString(command.Name)
	output.writeString(" [flags]")
	if command.Arguments.Maximum != 0 {
		output.writeByte(' ')
		writeArgumentHelp(output, command.Arguments)
	}
	output.writeString("\n\nFlags:\n")
	writeOptionHelp(output, command.Options, command.Name, true, false)
	if len(definition.options) != 0 || definition.version {
		output.writeString("\nGlobal Flags:\n")
		writeOptionHelp(output, definition.options, definition.name, false, definition.version)
	}
}

func writeUsagePrefix(output *helpOutput, application string) {
	output.writeString("  ")
	output.writeString(application)
	output.writeByte(' ')
}

func writeArgumentHelp(output *helpOutput, arguments Arguments) {
	if arguments.Maximum == 0 {
		return
	}
	if arguments.Minimum == 0 {
		output.writeByte('[')
	}
	output.writeString(arguments.Name)
	if arguments.Maximum > 1 {
		output.writeString("...")
	}
	if arguments.Minimum == 0 {
		output.writeByte(']')
	}
}

func commandWidth(definition *admittedDefinition) int {
	width := len(helpName)
	if definition.version {
		width = max(width, len(versionName))
	}
	for _, command := range definition.commands {
		width = max(width, len(command.Name))
	}
	return width
}

func writeHelpLine(output *helpOutput, label, summary string, width int) {
	output.writeString("  ")
	output.writeString(label)
	writeSpaces(output, width-len(label)+2)
	output.writeString(summary)
	output.writeByte('\n')
}

func writeOptionHelp(output *helpOutput, options []Option, owner string, includeHelp, includeVersion bool) {
	width := optionWidth(options, includeHelp, includeVersion)
	for _, option := range options {
		writeOptionLine(output, option, width)
	}
	if includeHelp {
		writeHelpLine(output, "-h, --help", "Show help for "+owner, width)
	}
	if includeVersion {
		writeHelpLine(output, "-v, --version", versionSummary, width)
	}
}

func optionWidth(options []Option, includeHelp, includeVersion bool) int {
	width := 0
	for _, option := range options {
		width = max(width, optionLabelLength(option))
	}
	if includeHelp {
		width = max(width, len("-h, --help"))
	}
	if includeVersion {
		width = max(width, len("-v, --version"))
	}
	return width
}

func writeOptionLine(output *helpOutput, option Option, width int) {
	output.writeString("  ")
	if option.Short == "" {
		output.writeString("    --")
	} else {
		output.writeByte('-')
		output.writeString(option.Short)
		output.writeString(", --")
	}
	output.writeString(option.Name)
	if option.Placeholder != "" {
		output.writeByte(' ')
		output.writeString(option.Placeholder)
	}
	writeSpaces(output, width-optionLabelLength(option)+2)
	output.writeString(option.Summary)
	switch {
	case option.Required:
		output.writeString(" (required)")
	case option.Kind == ValueBool && option.Default == "true":
		output.writeString(" (default true)")
	case option.Kind == ValueString && option.Default != "":
		output.writeString(" (default ")
		output.writeQuoted(option.Default)
		output.writeByte(')')
	}
	output.writeByte('\n')
}

func optionLabelLength(option Option) int {
	length := len("-x, --") + len(option.Name)
	if option.Placeholder != "" {
		length += 1 + len(option.Placeholder)
	}
	return length
}

func writeSpaces(output *helpOutput, count int) {
	for range count {
		output.writeByte(' ')
	}
}

func quotedLength(value string) int {
	length := 2
	for _, character := range value {
		if character == '"' || character == '\\' {
			length += 2
		} else {
			length += utf8.RuneLen(character)
		}
	}
	return length
}
