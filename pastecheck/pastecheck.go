// Package pastecheck provides the legacy clipboard verification command.
package pastecheck

import (
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/rob137/risper/desktop"
)

func Main(argv []string) int {
	parser := flag.NewFlagSet("risper paste-test", flag.ContinueOnError)
	parser.SetOutput(os.Stderr)
	if err := parser.Parse(argv); err != nil {
		if err == flag.ErrHelp {
			return 0
		}
		return 2
	}
	if parser.NArg() != 0 {
		fmt.Fprintln(os.Stderr, "risper paste-test: unexpected positional argument")
		return 2
	}

	marker := fmt.Sprintf("RISPER_PASTE_TEST_%d", time.Now().Unix())
	ok, message := desktop.CopyText(marker)
	if !ok {
		fmt.Fprintln(os.Stderr, "risper paste-test:", message)
		return 1
	}
	fmt.Printf("Copied marker %s (%s).\n", marker, message)
	fmt.Println("Paste it into the target text field to verify clipboard access.")
	return 0
}
