//go:build unix

package main

import (
	"os"
	"path/filepath"
	"syscall"
	"testing"
)

func TestUpdateEnvironmentFilePreservesOwnership(t *testing.T) {
	path := filepath.Join(t.TempDir(), "vrchat-logger.env")
	if err := os.WriteFile(path, []byte("VRC_AUTH_TOKEN=old\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	beforeInfo, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	before := beforeInfo.Sys().(*syscall.Stat_t)

	if err := updateEnvironmentFile(path, cookiePair{Auth: "dummy-auth", TwoFactor: "dummy-two-factor"}); err != nil {
		t.Fatal(err)
	}
	afterInfo, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	after := afterInfo.Sys().(*syscall.Stat_t)
	if before.Uid != after.Uid || before.Gid != after.Gid {
		t.Fatalf("ownership changed from %d:%d to %d:%d", before.Uid, before.Gid, after.Uid, after.Gid)
	}
}
