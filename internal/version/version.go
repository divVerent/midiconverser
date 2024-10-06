package version

import (
	"bytes"
	_ "embed"
)

//go:embed version.txt
var versionBytes []byte

// Version returns the version of this code.
func Version() string {
	return string(bytes.TrimSpace(versionBytes))
}
