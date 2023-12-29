package main

import (
	"os"

	"github.com/taylormonacelli/cutejaguar"
)

func main() {
	code := cutejaguar.Execute()
	os.Exit(code)
}
