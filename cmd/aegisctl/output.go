package main

import (
	"fmt"
	"os"
)

func checkOutput(_ int, err error) {
	if err != nil {
		fmt.Fprintln(os.Stderr, "Error: could not write output")
		os.Exit(1)
	}
}
