// Code generated by mockery v1.0.0. DO NOT EDIT.

package mocks

import mock "github.com/stretchr/testify/mock"
import tiling "go.skia.org/infra/go/tiling"
import types "go.skia.org/infra/golden/go/types"

// TileInfo is an autogenerated mock type for the TileInfo type
type TileInfo struct {
	mock.Mock
}

// AllCommits provides a mock function with given fields:
func (_m *TileInfo) AllCommits() []*tiling.Commit {
	ret := _m.Called()

	var r0 []*tiling.Commit
	if rf, ok := ret.Get(0).(func() []*tiling.Commit); ok {
		r0 = rf()
	} else {
		if ret.Get(0) != nil {
			r0 = ret.Get(0).([]*tiling.Commit)
		}
	}

	return r0
}

// DataCommits provides a mock function with given fields:
func (_m *TileInfo) DataCommits() []*tiling.Commit {
	ret := _m.Called()

	var r0 []*tiling.Commit
	if rf, ok := ret.Get(0).(func() []*tiling.Commit); ok {
		r0 = rf()
	} else {
		if ret.Get(0) != nil {
			r0 = ret.Get(0).([]*tiling.Commit)
		}
	}

	return r0
}

// GetTile provides a mock function with given fields: is
func (_m *TileInfo) GetTile(is types.IgnoreState) *tiling.Tile {
	ret := _m.Called(is)

	var r0 *tiling.Tile
	if rf, ok := ret.Get(0).(func(types.IgnoreState) *tiling.Tile); ok {
		r0 = rf(is)
	} else {
		if ret.Get(0) != nil {
			r0 = ret.Get(0).(*tiling.Tile)
		}
	}

	return r0
}
