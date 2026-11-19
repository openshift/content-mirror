//go:build !linux
// +build !linux

package reaper

// Start has no effect on non-linux platforms.
// Support for other unices will be added.
func Start() {
}
