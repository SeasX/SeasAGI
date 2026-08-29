package main

import (
	"os"

	"github.com/SeasAGI/SeasAGI-Server/platform-api/cmd"
	"github.com/SeasAGI/SeasAGI-Server/platform-api/internal/logging"
)

func main() {
	if err := cmd.Execute(); err != nil {
		logging.Fatal(err.Error())
		os.Exit(1)
	}
}
