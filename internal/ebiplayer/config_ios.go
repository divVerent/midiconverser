//go:build embed && ios

package ebiplayer

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"unsafe"

	"gopkg.in/yaml.v3"

	"github.com/divVerent/midiconverser/internal/file"
	"github.com/divVerent/midiconverser/internal/processor"
)

/*
#cgo CFLAGS: -x objective-c
#cgo LDFLAGS: -framework Foundation

#import <Foundation/NSPathUtilities.h>
#import <Foundation/NSString.h>

#include <stdlib.h>
#include <string.h>

const char *configRoot() {
	NSArray<NSString *> *paths = NSSearchPathForDirectoriesInDomains(
		NSApplicationSupportDirectory, NSUserDomainMask, YES);
	if ([paths count] < 1) {
		return NULL;
	}
	NSString *path = [paths firstObject];
	if (path == nil) {
		return NULL;
	}
	const char *data = [path UTF8String];
	if (data == NULL) {
		return NULL;
	}
	return strdup(data);
}
*/
import "C"

var configRoot string

func init() {
	configRootC := C.configRoot()
	if configRootC == nil {
		panic("could not find config location")
	}
	configRoot = filepath.Join(C.GoString(configRootC), "MIDIConverser")
	C.free(unsafe.Pointer(configRootC))
}

func loadConfig(fsys fs.FS, name string) (*processor.Config, error) {
	return file.ReadConfig(fsys, name)
}

func loadConfigOverride(name string, into *processor.Config) error {
	config, err := file.ReadConfig(os.DirFS(configRoot), name)
	if err != nil {
		return err
	}
	copyConfigOverrideFields(config, into)
	return nil
}

func saveConfigOverride(name string, config *processor.Config) (err error) {
	var subset processor.Config
	copyConfigOverrideFields(config, &subset)
	f, err := os.Create(filepath.Join(configRoot, name))
	if err != nil {
		return fmt.Errorf("could not recreate: %v", err)
	}
	defer func() {
		closeErr := f.Close()
		if closeErr != nil && err == nil {
			err = closeErr
		}
	}()
	enc := yaml.NewEncoder(f)
	enc.SetIndent(2) // Match yq.
	return enc.Encode(subset)
}
