//go:build unix

package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"syscall"
)

func preserveFileOwnership(file *os.File, original os.FileInfo) error {
	stat, ok := original.Sys().(*syscall.Stat_t)
	if !ok {
		return errors.New("read environment file ownership")
	}
	if err := file.Chown(int(stat.Uid), int(stat.Gid)); err != nil {
		return fmt.Errorf("preserve environment file ownership: %w", err)
	}
	return nil
}

func syncParentDirectory(path string) error {
	dir, err := os.Open(filepath.Dir(path))
	if err != nil {
		return err
	}
	defer dir.Close()
	if err := dir.Sync(); err != nil {
		return fmt.Errorf("sync environment file directory: %w", err)
	}
	return nil
}
