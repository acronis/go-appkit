package throttle

import (
	"testing"

	"github.com/stretchr/testify/suite"
)

func TestThrottleUnaryInterceptor(t *testing.T) {
	suite.Run(t, &ThrottleInterceptorTestSuite{IsUnary: true})
}
