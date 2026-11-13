package mover_shutdown

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestMoverShutdown(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Mover shutdown suite")
}
