package main

import (
	"os"

	"github.com/200iq-creature/KVK-panel/internal/app"
)

func main() {
	os.Exit(app.Main(os.Args[1:]))
}
