package image_test

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	
	"github.com/lissto-dev/api/pkg/logging"
	"go.uber.org/zap"
)

func TestImage(t *testing.T) {
	// Initialize logger for tests
	logger, _ := zap.NewDevelopment()
	logging.Logger = logger
	
	RegisterFailHandler(Fail)
	RunSpecs(t, "Image Suite")
}
