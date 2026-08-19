// Package args contains the small bit of argument normalization needed by
// Go's standard flag package. Python's argparse accepts options after a
// positional argument; the Go commands keep that useful CLI shape.
package args

import "strings"

func Reorder(values []string, valueFlags map[string]bool) []string {
	flags := make([]string, 0, len(values))
	positionals := make([]string, 0, len(values))
	for index := 0; index < len(values); index++ {
		value := values[index]
		if !strings.HasPrefix(value, "-") || value == "-" {
			positionals = append(positionals, value)
			continue
		}
		flags = append(flags, value)
		name := value
		if equal := strings.IndexByte(name, '='); equal >= 0 {
			name = name[:equal]
		}
		if valueFlags[name] && !strings.Contains(value, "=") && index+1 < len(values) {
			index++
			flags = append(flags, values[index])
		}
	}
	return append(flags, positionals...)
}
