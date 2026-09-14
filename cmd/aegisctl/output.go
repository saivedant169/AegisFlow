package main

import (
	"fmt"
	"os"
)

func checkOutput(_ int, err error) {
	if err != nil {
		_, err := fmt.Fprintln(os.Stderr, "Error: could not write output")
		if err != nil {
			return
		}
		os.Exit(1)
	}
}
