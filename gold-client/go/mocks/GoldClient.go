// Code generated by mockery v1.0.0. DO NOT EDIT.

package mocks

import jsonio "go.skia.org/infra/golden/go/jsonio"
import mock "github.com/stretchr/testify/mock"
import types "go.skia.org/infra/golden/go/types"

// GoldClient is an autogenerated mock type for the GoldClient type
type GoldClient struct {
	mock.Mock
}

// Finalize provides a mock function with given fields:
func (_m *GoldClient) Finalize() error {
	ret := _m.Called()

	var r0 error
	if rf, ok := ret.Get(0).(func() error); ok {
		r0 = rf()
	} else {
		r0 = ret.Error(0)
	}

	return r0
}

// SetSharedConfig provides a mock function with given fields: sharedConfig
func (_m *GoldClient) SetSharedConfig(sharedConfig jsonio.GoldResults) error {
	ret := _m.Called(sharedConfig)

	var r0 error
	if rf, ok := ret.Get(0).(func(jsonio.GoldResults) error); ok {
		r0 = rf(sharedConfig)
	} else {
		r0 = ret.Error(0)
	}

	return r0
}

// Test provides a mock function with given fields: name, imgFileName, additionalKeys
func (_m *GoldClient) Test(name types.TestName, imgFileName string, additionalKeys map[string]string) (bool, error) {
	ret := _m.Called(name, imgFileName, additionalKeys)

	var r0 bool
	if rf, ok := ret.Get(0).(func(types.TestName, string, map[string]string) bool); ok {
		r0 = rf(name, imgFileName, additionalKeys)
	} else {
		r0 = ret.Get(0).(bool)
	}

	var r1 error
	if rf, ok := ret.Get(1).(func(types.TestName, string, map[string]string) error); ok {
		r1 = rf(name, imgFileName, additionalKeys)
	} else {
		r1 = ret.Error(1)
	}

	return r0, r1
}
