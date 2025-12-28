package postprocessor_test

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/lissto-dev/api/pkg/logging"
)

func TestPostprocessor(t *testing.T) {
	// Initialize logger for tests
	_ = logging.InitLogger("info", "console")

	RegisterFailHandler(Fail)
	RunSpecs(t, "Postprocessor Suite")
}
