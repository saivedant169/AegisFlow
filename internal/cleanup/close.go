// Package cleanup reports best-effort resource teardown failures.
package cleanup

import (
	"io"
	"log"
)

// Close releases a resource after its operation has completed. Read, write,
// transaction, and persistence errors must be handled at their call sites.
func Close(resource io.Closer) {
	if err := resource.Close(); err != nil {
		log.Print("resource cleanup failed")
	}
}
