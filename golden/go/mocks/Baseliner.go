// Code generated by mockery v1.0.0. DO NOT EDIT.

package mocks

import baseline "go.skia.org/infra/golden/go/baseline"
import digest_counter "go.skia.org/infra/golden/go/digest_counter"
import mock "github.com/stretchr/testify/mock"

// Baseliner is an autogenerated mock type for the Baseliner type
type Baseliner struct {
	mock.Mock
}

// CanWriteBaseline provides a mock function with given fields:
func (_m *Baseliner) CanWriteBaseline() bool {
	ret := _m.Called()

	var r0 bool
	if rf, ok := ret.Get(0).(func() bool); ok {
		r0 = rf()
	} else {
		r0 = ret.Get(0).(bool)
	}

	return r0
}

// FetchBaseline provides a mock function with given fields: commitHash, issueID, issueOnly
func (_m *Baseliner) FetchBaseline(commitHash string, issueID int64, issueOnly bool) (*baseline.Baseline, error) {
	ret := _m.Called(commitHash, issueID, issueOnly)

	var r0 *baseline.Baseline
	if rf, ok := ret.Get(0).(func(string, int64, bool) *baseline.Baseline); ok {
		r0 = rf(commitHash, issueID, issueOnly)
	} else {
		if ret.Get(0) != nil {
			r0 = ret.Get(0).(*baseline.Baseline)
		}
	}

	var r1 error
	if rf, ok := ret.Get(1).(func(string, int64, bool) error); ok {
		r1 = rf(commitHash, issueID, issueOnly)
	} else {
		r1 = ret.Error(1)
	}

	return r0, r1
}

// PushIssueBaseline provides a mock function with given fields: issueID, tileInfo, dCounter
func (_m *Baseliner) PushIssueBaseline(issueID int64, tileInfo baseline.TileInfo, dCounter digest_counter.DigestCounter) error {
	ret := _m.Called(issueID, tileInfo, dCounter)

	var r0 error
	if rf, ok := ret.Get(0).(func(int64, baseline.TileInfo, digest_counter.DigestCounter) error); ok {
		r0 = rf(issueID, tileInfo, dCounter)
	} else {
		r0 = ret.Error(0)
	}

	return r0
}

// PushMasterBaselines provides a mock function with given fields: tileInfo, targetHash
func (_m *Baseliner) PushMasterBaselines(tileInfo baseline.TileInfo, targetHash string) (*baseline.Baseline, error) {
	ret := _m.Called(tileInfo, targetHash)

	var r0 *baseline.Baseline
	if rf, ok := ret.Get(0).(func(baseline.TileInfo, string) *baseline.Baseline); ok {
		r0 = rf(tileInfo, targetHash)
	} else {
		if ret.Get(0) != nil {
			r0 = ret.Get(0).(*baseline.Baseline)
		}
	}

	var r1 error
	if rf, ok := ret.Get(1).(func(baseline.TileInfo, string) error); ok {
		r1 = rf(tileInfo, targetHash)
	} else {
		r1 = ret.Error(1)
	}

	return r0, r1
}
