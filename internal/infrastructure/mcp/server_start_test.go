package mcp

import (
	"context"
	"errors"
	"testing"

	"github.com/felixgeelhaar/roady/pkg/storage"
)

func TestServerServeHTTPReturnsCanceled(t *testing.T) {
	root := t.TempDir()
	repo := storage.NewFilesystemRepository(root)
	if err := repo.Initialize(); err != nil {
		t.Fatalf("initialize repo: %v", err)
	}
	if err := initProjectDir(root); err != nil {
		t.Fatalf("init mock AI config: %v", err)
	}

	server, err := NewServer(root)
	if err != nil {
		t.Fatalf("create server: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	if err := server.ServeHTTP(ctx, "127.0.0.1:0"); !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context.Canceled, got %v", err)
	}
}
