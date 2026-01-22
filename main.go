package main

import (
	"github.com/xulei1234/x-agent/cmd"
	"os"
)

func main() {
	if err := cmd.Execute(); err != nil {
		os.Exit(1)
	}
}
