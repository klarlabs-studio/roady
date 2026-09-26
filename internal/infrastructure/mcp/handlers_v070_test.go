package mcp

import (
	"context"
	"os"
	"testing"
)

// initProjectDir creates the .roady directory a handler expects to find.
func initProjectDir(root string) error {
	return os.MkdirAll(root+"/.roady", 0755)
}

func TestServer_HandleSmartDecompose_NoService(t *testing.T) {
	server := &Server{root: t.TempDir()}
	res, err := server.handleSmartDecompose(context.Background(), SmartDecomposeArgs{})
	assertToolError(t, res, err, "")
}
