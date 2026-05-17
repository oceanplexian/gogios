//go:build !linux

package main

// Stub for non-Linux builds (e.g. macOS dev). The shim is only meaningful
// on Linux; this lets the rest of the module compile cross-platform.
func run(command string) int { return 126 }
