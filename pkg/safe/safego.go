package safe

import (
	"log"
	"runtime/debug"
)

// Go executes fn in a separate goroutine and catches any panic to prevent the entire service from crashing.
func Go(fn func()) {
	go func() {
		defer func() {
			if r := recover(); r != nil {
				log.Printf("🚨 [CRITICAL GOROUTINE PANIC RECOVERED] %v\nStack trace:\n%s", r, debug.Stack())
			}
		}()
		fn()
	}()
}

