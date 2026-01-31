package testutils

import (
	"path/filepath"
	"runtime"

	"github.com/containerd/nerdctl/mod/tigron/test"

	"github.com/mycophonic/agar/pkg/agar"
)

func projectRoot() string {
	_, thisFile, _, _ := runtime.Caller(0) //nolint:dogsled // runtime.Caller returns 4 values, only file is needed

	return filepath.Dir(filepath.Dir(filepath.Dir(thisFile)))
}

// BinaryPath returns the absolute path to the saprobe binary.
func BinaryPath() string {
	return filepath.Join(projectRoot(), "bin", "saprobe")
}

// Setup creates a test case configured to run the saprobe binary.
func Setup() *test.Case {
	return agar.Setup(BinaryPath())
}
