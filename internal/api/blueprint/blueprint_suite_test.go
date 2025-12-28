package blueprint_test

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestBlueprint(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Blueprint Suite")
}
