package main

import (
	"os"

	"github.com/denysvitali/grok-proxy/cmd"
)

func main() {
	os.Exit(cmd.Execute())
}
