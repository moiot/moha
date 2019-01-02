package postgresql_test

import (
	"testing"

	. "gopkg.in/check.v1"
)

type testPostgreSQLSuite struct{}

var _ = Suite(&testPostgreSQLSuite{})

// Hock into "go test" runner.
func TestMySQL(t *testing.T) {
	TestingT(t)
}

func (s *testPostgreSQLSuite) Test_Xxx(c *C) {
}
