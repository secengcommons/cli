package cli_test

import (
	"fmt"

	"github.com/secengcommons/cli"
)

func Example() {
	parser, err := cli.New(cli.Definition{
		Name: "check",
		Options: []cli.Option{{
			Name: "root", Short: "r", Placeholder: "PATH",
			Summary: "Repository root", Kind: cli.ValueString, Default: ".",
		}},
		Commands: []cli.Command{{Name: "static", Summary: "Run static checks"}},
	})
	if err != nil {
		fmt.Println("definition rejected")
		return
	}

	invocation, err := parser.Parse([]string{"--root", "repository", "static"})
	if err != nil {
		fmt.Println("invocation rejected")
		return
	}
	root, found := invocation.String("root")
	if !found {
		fmt.Println("root option unavailable")
		return
	}
	fmt.Println(invocation.Command(), root)
	// Output:
	// static repository
}
