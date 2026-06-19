package debuglog

import (
	"log"
	"os"
	"strings"
)

// enabled is set once at startup. Set LOG_LEVEL=debug to activate.
var enabled = strings.EqualFold(strings.TrimSpace(os.Getenv("LOG_LEVEL")), "debug")

// Printf writes a debug-level log line. It is a no-op unless LOG_LEVEL=debug.
func Printf(format string, args ...any) {
	if enabled {
		log.Printf("[DEBUG] "+format, args...)
	}
}
