package crypt

import (
	"testing"

	"github.com/RaiseOfEASV/Cerberus/apps/vuln-api/internal/crypt/crypttest"
)

func newFakeForTest(t *testing.T) Transit { t.Helper(); return crypttest.New() }
