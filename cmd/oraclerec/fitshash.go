package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
)

const (
	fitsBlock = 2880
	fitsCard  = 80
)

// fitsDataSHA256 hashes everything after the primary header (the data unit and any extension):
// Siril stamps DATE, PROGRAM and HISTORY into every header it writes, so whole-file hashes change
// on every run while the pixels do not.
func fitsDataSHA256(path string) (string, error) {
	body, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	start, err := fitsDataOffset(body)
	if err != nil {
		return "", fmt.Errorf("%s: %w", path, err)
	}
	sum := sha256.Sum256(body[start:])
	return hex.EncodeToString(sum[:]), nil
}

// fitsDataOffset returns the offset of the first byte after the primary header: the block that
// follows the one holding the END card.
func fitsDataOffset(body []byte) (int, error) {
	end := []byte("END" + string(bytes.Repeat([]byte(" "), fitsCard-3)))
	for off := 0; off+fitsCard <= len(body); off += fitsCard {
		if bytes.Equal(body[off:off+fitsCard], end) {
			next := (off/fitsBlock + 1) * fitsBlock
			if next > len(body) {
				return 0, fmt.Errorf("header ends past the file")
			}
			return next, nil
		}
	}
	return 0, fmt.Errorf("no END card")
}
