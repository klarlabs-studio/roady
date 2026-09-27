package mcp

import (
	"os"
)

// initProjectDir creates the .roady directory a handler expects to find.
func initProjectDir(root string) error {
	return os.MkdirAll(root+"/.roady", 0755)
}
