// Code generated by "stringer -type=AlertFilter"; DO NOT EDIT.

package alertfilter

import "fmt"

const _AlertFilter_name = "ALLOWNEREOL"

var _AlertFilter_index = [...]uint8{0, 3, 8, 11}

func (i AlertFilter) String() string {
	if i < 0 || i >= AlertFilter(len(_AlertFilter_index)-1) {
		return fmt.Sprintf("AlertFilter(%d)", i)
	}
	return _AlertFilter_name[_AlertFilter_index[i]:_AlertFilter_index[i+1]]
}