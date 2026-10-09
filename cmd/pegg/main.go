package main

import (
	"fmt"
	"os"

	"github.com/peggco/pegg/internal/core/bootstrap"
)

func main() {
	if err := bootstrap.Run(os.Args[1:]); err != nil {
		fmt.Fprintf(os.Stderr, "pegg: %v\n", err)
		os.Exit(1)
	}
}
