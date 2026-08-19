package main

import (
	"os"

	"github.com/rob137/risper/toggle"
)

func main() {
	os.Exit(toggle.Main(os.Args[1:]))
}
