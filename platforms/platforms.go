// Package platforms contains the small OS boundary used by the daemon.
//
// This phase intentionally implements only the Linux input path that exists
// on the supported machine. The interface leaves room for another adapter
// without carrying unimplemented macOS or Windows code into the rewrite.
package platforms

import "runtime"

type DoubleAltListener interface {
	Start() (bool, string)
	Stop()
	SetLogger(func(string))
}

func IsLinux() bool { return runtime.GOOS == "linux" }
