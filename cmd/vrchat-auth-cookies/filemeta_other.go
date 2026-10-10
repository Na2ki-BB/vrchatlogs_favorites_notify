//go:build !unix

package main

import "os"

func preserveFileOwnership(_ *os.File, _ os.FileInfo) error {
	return nil
}

func syncParentDirectory(_ string) error {
	return nil
}
