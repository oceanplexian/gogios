// gogios-runcheck is a tiny shim that runs a single check plugin inside a
// fresh PID namespace. When the shim is killed (e.g. by gogios's check
// timeout), the kernel atomically tears down the namespace and reaps every
// descendant — making it structurally impossible for plugins like fping to
// be reparented to the container's PID 1 and accumulate as zombies.
//
// Invocation: gogios-runcheck <shell-command-string>
// Exit: same code as the inner shell.
package main

import (
	"fmt"
	"os"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, "usage: gogios-runcheck <shell-command>")
		os.Exit(2)
	}
	os.Exit(run(os.Args[1]))
}
