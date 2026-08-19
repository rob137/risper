// Package cli implements the service-management command that was historically
// exposed as the top-level Python `risper` entry point.
package cli

import (
	"flag"
	"fmt"
	"os"
	"os/exec"
)

const ServiceName = "risper.service"

func Main(args []string) int {
	parser := flag.NewFlagSet("risper", flag.ContinueOnError)
	parser.SetOutput(os.Stderr)
	if err := parser.Parse(args); err != nil {
		if err == flag.ErrHelp {
			return 0
		}
		return 2
	}
	if parser.NArg() > 1 {
		fmt.Fprintln(os.Stderr, "risper: unexpected positional arguments")
		return 2
	}
	action := "start"
	if parser.NArg() == 1 {
		action = parser.Arg(0)
	}
	switch action {
	case "start":
		code := systemctl("enable", "--now", ServiceName)
		if code == 0 {
			fmt.Println("Risper daemon enabled and running.")
		}
		return code
	case "kill", "stop":
		code := systemctl("stop", ServiceName)
		if code == 0 {
			fmt.Println("Risper daemon stopped. Autostart remains enabled.")
		}
		return code
	case "restart":
		code := systemctl("restart", ServiceName)
		if code == 0 {
			fmt.Println("Risper daemon restarted.")
		}
		return code
	case "status":
		return systemctl("status", ServiceName, "--no-pager")
	default:
		fmt.Fprintf(os.Stderr, "risper: invalid action %q (want start, kill, stop, restart, or status)\n", action)
		return 2
	}
}

func systemctl(args ...string) int {
	command := exec.Command("systemctl", append([]string{"--user"}, args...)...)
	command.Stdin = os.Stdin
	command.Stdout = os.Stdout
	command.Stderr = os.Stderr
	if err := command.Run(); err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			return exitErr.ExitCode()
		}
		fmt.Fprintln(os.Stderr, "risper:", err)
		return 1
	}
	return 0
}
