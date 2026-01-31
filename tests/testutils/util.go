package testutils

import (
	"github.com/containerd/nerdctl/mod/tigron/test"

	"github.com/mycophonic/agar/pkg/agar"
)

// Setup creates a test case configured to run the saprobe binary.
func Setup() *test.Case {
	return agar.Setup("saprobe")
}
