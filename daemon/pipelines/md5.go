package pipelines

import (
	"crypto/md5"
	"encoding/hex"
	"io"
	"os"
)

// MD5File returns the lowercase hex MD5 of the file's contents. Used
// by game-disc pipelines to verify a rip against the Redump dat hash.
func MD5File(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer func() { _ = f.Close() }()
	h := md5.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}
