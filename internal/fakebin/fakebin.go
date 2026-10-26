// Package fakebin writes the shell scripts tests use as fake CLIs.
//
// macOS checks every new executable the first time it runs (150-300ms each),
// and tests write hundreds of them. So the script itself is not executable: the
// fake CLI is a symlink to one shared launcher, checked once per machine,
// which runs "<its path>.sh" with /bin/sh.
package fakebin

import (
	"os"
	"path/filepath"
	"sync"
)

const launcher = "#!/bin/sh\nexec /bin/sh \"$0.sh\" \"$@\"\n"

var (
	once sync.Once
	path string
	err  error
)

// Write makes path run script, like os.WriteFile(path, script, 0o755).
func Write(name string, script []byte) error {
	once.Do(func() {
		path = filepath.Join(os.TempDir(), "agent-office-fakebin")
		if b, e := os.ReadFile(path); e == nil && string(b) == launcher {
			return
		}
		tmp := path + "." + filepath.Base(os.Args[0])
		if err = os.WriteFile(tmp, []byte(launcher), 0o755); err == nil {
			err = os.Rename(tmp, path) // packages test in parallel: atomic
		}
	})
	if err != nil {
		return err
	}
	if err := os.WriteFile(name+".sh", script, 0o644); err != nil {
		return err
	}
	if target, e := os.Readlink(name); e == nil && target == path {
		return nil // rewritten mid-test
	}
	os.Remove(name)
	return os.Symlink(path, name)
}
