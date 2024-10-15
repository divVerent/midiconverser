//go:build embed && age

package main

import (
	"archive/zip"
	"bytes"
	_ "embed"
	"filippo.io/age"
	"fmt"
	"io"
	"io/fs"
)

//go:embed vfs.zip.age
var vfsCiphertext []byte

func openFS(pw string) (fs.FS, error) {
	id, err := age.NewScryptIdentity(pw)
	if err != nil {
		return nil, fmt.Errorf("could not build scrypt identity: %w", err)
	}
	vfsPlaintextReader, err := age.Decrypt(bytes.NewReader(vfsCiphertext), id)
	if err != nil {
		return nil, fmt.Errorf("could not start decrypting: %w", err)
	}
	vfsPlaintext, err := io.ReadAll(vfsPlaintextReader)
	if err != nil {
		return nil, fmt.Errorf("could not finish decrypting: %w", err)
	}
	return zip.NewReader(bytes.NewReader(vfsPlaintext), int64(len(vfsPlaintext)))
}
