package ipc

import (
	"context"
	"encoding/json"
	"path/filepath"
	"testing"
	"time"
)

func TestServerClientRoundTrip(t *testing.T) {
	dir := t.TempDir()
	sock := filepath.Join(dir, "meetbar.sock")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	srv := NewServer(sock, func(ctx context.Context, method string, params json.RawMessage) (any, error) {
		if method != "status" {
			t.Fatalf("unexpected method %s", method)
		}
		return StatusResult{Daemon: "ok", Auth: "disconnected"}, nil
	})
	if err := srv.Start(ctx); err != nil {
		t.Fatal(err)
	}

	// Wait briefly for listen
	time.Sleep(20 * time.Millisecond)

	client := NewClient(sock)
	res, err := CallDecode[StatusResult](client, "status", nil)
	if err != nil {
		t.Fatal(err)
	}
	if res.Daemon != "ok" || res.Auth != "disconnected" {
		t.Fatalf("unexpected %#v", res)
	}
	cancel()
	srv.Wait()
}
