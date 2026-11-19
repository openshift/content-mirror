//go:build linux
// +build linux

package reaper

import (
	"os"
	"os/signal"
	"syscall"
)

// Start starts a goroutine to reap processes if called from a process
// that has pid 1.
func Start() {
	if os.Getpid() == 1 {
		go func() {
			sigs := make(chan os.Signal, 1)
			signal.Notify(sigs, syscall.SIGCHLD)
			for {
				// Wait for a child to terminate
				<-sigs
				for {
					// Reap processes
					cpid, _ := syscall.Wait4(-1, nil, syscall.WNOHANG, nil)
					if cpid < 1 {
						break
					}
				}
			}
		}()
	}
}
