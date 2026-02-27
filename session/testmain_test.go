package session

import (
	"os"
	"testing"

	"github.com/kastheco/kasmos/log"
)

func TestMain(m *testing.M) {
	log.Initialize(false)
	code := m.Run()
	log.Close()
	os.Exit(code)
}
