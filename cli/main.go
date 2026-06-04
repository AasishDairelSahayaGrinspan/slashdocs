package main

import (
	"fmt"
	"os"

	"github.com/razorpay/aidocs/cli/internal"
)

func main() {
	if err := internal.NewRoot(os.Stdout).Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(internal.ExitCode(err))
	}
}
