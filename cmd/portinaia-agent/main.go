package main

import (
	"fmt"
	"os"

	"portinaia/internal/version"
)

func main() {
	if len(os.Args) != 2 || os.Args[1] != "version" {
		fmt.Fprintln(os.Stderr, "usage: portinaia-agent version")
		os.Exit(2)
	}

	fmt.Printf("portinaia-agent %s\n", version.String())
}
