package file

import (
	"bytes"
	"crypto/sha256"
	"fmt"
	"io/fs"

	"gitlab.com/gomidi/midi/v2/smf"

	"github.com/divVerent/midiconverser/internal/processor"
)

// Process processes the given options file. May mutate options - if so, main program may want to write it back.
func Process(fsys fs.FS, config *processor.Config, options *processor.Options) (map[processor.OutputKey]*smf.SMF, error) {
	inBytes, err := fs.ReadFile(fsys, options.InputFile)
	if err != nil {
		return nil, fmt.Errorf("could not read %v: %v", options.InputFile, err)
	}

	sum := fmt.Sprintf("%x", sha256.Sum256(inBytes))

	if options.InputFileSHA256 != "" && options.InputFileSHA256 != sum {
		return nil, fmt.Errorf("mismatching checksum of %v: got %v, want %v", options.InputFile, sum, options.InputFileSHA256)
	}
	options.InputFileSHA256 = sum

	in, err := smf.ReadFrom(bytes.NewReader(inBytes))
	if err != nil {
		return nil, fmt.Errorf("could not parse %v: %v", options.InputFile, err)
	}

	output, err := processor.Process(in, config, options)
	if err != nil {
		return nil, fmt.Errorf("failed to process %v: %v", options.InputFile, err)
	}

	return output, nil
}
