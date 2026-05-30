package main

import (
	"os"

	"dx-util-env-manager/internal/app"
)

func main() {
	os.Exit(app.Run(os.Args, os.Stdout, os.Stderr))
}
