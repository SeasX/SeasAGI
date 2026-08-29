package main

import (
	"os"

	"github.com/SeasAGI/SeasAGI-Server/relay-gateway/cmd"
	"github.com/SeasAGI/SeasAGI-Server/relay-gateway/internal/logging"
)

func main() {
	if err := cmd.Execute(); err != nil {
		logging.Fatalf("%v", err)
		os.Exit(1)
	}
}
