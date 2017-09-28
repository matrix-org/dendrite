package mocks

import "github.com/uber/tchannel-go/thrift/gen-go/meta"
import "github.com/stretchr/testify/mock"

import "github.com/uber/tchannel-go/thrift"

type TChanMeta struct {
	mock.Mock
}

func (_m *TChanMeta) Health(ctx thrift.Context) (*meta.HealthStatus, error) {
	ret := _m.Called(ctx)

	var r0 *meta.HealthStatus
	if rf, ok := ret.Get(0).(func(thrift.Context) *meta.HealthStatus); ok {
		r0 = rf(ctx)
	} else {
		if ret.Get(0) != nil {
			r0 = ret.Get(0).(*meta.HealthStatus)
		}
	}

	var r1 error
	if rf, ok := ret.Get(1).(func(thrift.Context) error); ok {
		r1 = rf(ctx)
	} else {
		r1 = ret.Error(1)
	}

	return r0, r1
}
