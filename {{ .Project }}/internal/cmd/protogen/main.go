package main

import (
	"fmt"
	"os"
)

func main() {
	if err := run("."); err != nil {
		fmt.Fprintln(os.Stderr, "protogen:", err)
		os.Exit(1)
	}
}
