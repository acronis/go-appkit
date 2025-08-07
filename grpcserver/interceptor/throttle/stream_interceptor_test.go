package throttle

import (
	"testing"

	"github.com/stretchr/testify/suite"
)

func TestThrottleStreamInterceptor(t *testing.T) {
	suite.Run(t, &ThrottleInterceptorTestSuite{IsUnary: false})
}
