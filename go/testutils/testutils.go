// Convenience utilities for testing.
package testutils

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"path"
	"runtime"
	"strings"
	"sync"
	"time"

	assert "github.com/stretchr/testify/require"
	"go.skia.org/infra/go/repo_root"
	"go.skia.org/infra/go/sktest"
)

var (
	// TryAgainErr use used by TryUntil.
	TryAgainErr = errors.New("Trying Again")
)

// TestDataDir returns the path to the caller's testdata directory, which
// is assumed to be "<path to caller dir>/testdata".
func TestDataDir() (string, error) {
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		return "", fmt.Errorf("Could not find test data dir: runtime.Caller() failed.")
	}
	for skip := 0; ; skip++ {
		_, file, _, ok := runtime.Caller(skip)
		if !ok {
			return "", fmt.Errorf("Could not find test data dir: runtime.Caller() failed.")
		}
		if file != thisFile {
			return path.Join(path.Dir(file), "testdata"), nil
		}
	}
}

func readFile(filename string) (io.Reader, error) {
	dir, err := TestDataDir()
	if err != nil {
		return nil, fmt.Errorf("Could not read %s: %v", filename, err)
	}
	f, err := os.Open(path.Join(dir, filename))
	if err != nil {
		return nil, fmt.Errorf("Could not read %s: %v", filename, err)
	}
	return f, nil
}

// ReadFile reads a file from the caller's testdata directory.
func ReadFile(filename string) (string, error) {
	f, err := readFile(filename)
	if err != nil {
		return "", err
	}
	b, err := ioutil.ReadAll(f)
	if err != nil {
		return "", fmt.Errorf("Could not read %s: %v", filename, err)
	}
	return string(b), nil
}

// MustReadFile reads a file from the caller's testdata directory and panics on
// error.
func MustReadFile(filename string) string {
	s, err := ReadFile(filename)
	if err != nil {
		panic(err)
	}
	return s
}

// ReadJsonFile reads a JSON file from the caller's testdata directory into the
// given interface.
func ReadJsonFile(filename string, dest interface{}) error {
	f, err := readFile(filename)
	if err != nil {
		return err
	}
	return json.NewDecoder(f).Decode(dest)
}

// MustReadJsonFile reads a JSON file from the caller's testdata directory into
// the given interface and panics on error.
func MustReadJsonFile(filename string, dest interface{}) {
	if err := ReadJsonFile(filename, dest); err != nil {
		panic(err)
	}
}

// WriteFile writes the given contents to the given file path, reporting any
// error.
func WriteFile(t sktest.TestingT, filename, contents string) {
	assert.NoErrorf(t, ioutil.WriteFile(filename, []byte(contents), os.ModePerm), "Unable to write to file %s", filename)
}

// AssertCloses takes an ioutil.Closer and asserts that it closes. E.g.:
// frobber := NewFrobber()
// defer testutils.AssertCloses(t, frobber)
func AssertCloses(t sktest.TestingT, c io.Closer) {
	assert.NoError(t, c.Close())
}

// Remove attempts to remove the given file and asserts that no error is returned.
func Remove(t sktest.TestingT, fp string) {
	assert.NoError(t, os.Remove(fp))
}

// RemoveAll attempts to remove the given directory and asserts that no error is returned.
func RemoveAll(t sktest.TestingT, fp string) {
	assert.NoError(t, os.RemoveAll(fp))
}

// TempDir is a wrapper for ioutil.TempDir. Returns the path to the directory and a cleanup
// function to defer.
func TempDir(t sktest.TestingT) (string, func()) {
	d, err := ioutil.TempDir("", "testutils")
	assert.NoError(t, err)
	return d, func() {
		RemoveAll(t, d)
	}
}

// MarshalJSON encodes the given interface to a JSON string.
func MarshalJSON(t sktest.TestingT, i interface{}) string {
	b, err := json.Marshal(i)
	assert.NoError(t, err)
	return string(b)
}

// MarshalIndentJSON encodes the given interface to an indented JSON string.
func MarshalIndentJSON(t sktest.TestingT, i interface{}) string {
	b, err := json.MarshalIndent(i, "", "  ")
	assert.NoError(t, err)
	return string(b)
}

// AssertErrorContains asserts that the given error contains the given string.
func AssertErrorContains(t sktest.TestingT, err error, substr string) {
	assert.NotNil(t, err)
	assert.True(t, strings.Contains(err.Error(), substr))
}

// Return the path to the root of the checkout.
func GetRepoRoot(t sktest.TestingT) string {
	root, err := repo_root.Get()
	assert.NoError(t, err)
	return root
}

// EventuallyConsistent tries a test repeatedly until either the test passes
// or time expires, and is used when tests are written to expect
// non-eventual consistency.
//
// Use this function sparingly.
//
// duration - The amount of time to keep trying.
// f - The func to run the tests, should return TryAgainErr if
//     we should keep trying, otherwise TryUntil will return
//     with the err that f() returns.
func EventuallyConsistent(duration time.Duration, f func() error) error {
	begin := time.Now()
	for time.Now().Sub(begin) < duration {
		if err := f(); err != TryAgainErr {
			return err
		}
	}
	return fmt.Errorf("Failed to pass test in allotted time.")
}

// MockTestingT implements sktest.TestingT by saving calls to Log and Fail. MockTestingT can
// be used to test a test helper function. See also AssertFails.
// The methods Helper, Name, Skip, SkipNow, Skipf, and Skipped are unimplemented.
// This type is not safe for concurrent use.
type MockTestingT struct {
	LogMsgs  []string
	IsFailed bool
}

func (m *MockTestingT) Error(args ...interface{}) {
	m.Log(args...)
	m.Fail()
}
func (m *MockTestingT) Errorf(format string, args ...interface{}) {
	m.Logf(format, args...)
	m.Fail()
}
func (m *MockTestingT) Fail() {
	m.IsFailed = true
}
func (m *MockTestingT) FailNow() {
	m.Fail()
	runtime.Goexit()
}
func (m *MockTestingT) Failed() bool {
	return m.IsFailed
}
func (m *MockTestingT) Fatal(args ...interface{}) {
	m.Log(args...)
	m.FailNow()
}
func (m *MockTestingT) Fatalf(format string, args ...interface{}) {
	m.Logf(format, args...)
	m.FailNow()
}
func (m *MockTestingT) Helper() {}
func (m *MockTestingT) Log(args ...interface{}) {
	m.LogMsgs = append(m.LogMsgs, fmt.Sprintln(args...))
}
func (m *MockTestingT) Logf(format string, args ...interface{}) {
	m.LogMsgs = append(m.LogMsgs, fmt.Sprintf(format, args...))
}
func (m *MockTestingT) Name() string {
	return ""
}
func (m *MockTestingT) Skip(args ...interface{}) {
	m.Log(args...)
	m.SkipNow()
}
func (m *MockTestingT) SkipNow() {
	panic("SkipNow is not implemented.")
}
func (m *MockTestingT) Skipf(format string, args ...interface{}) {
	m.Logf(format, args...)
	m.SkipNow()
}
func (m *MockTestingT) Skipped() bool {
	return false
}

// Assert that MockTestingT implements the sktest.TestingT interface:
var _ sktest.TestingT = (*MockTestingT)(nil)

// AssertFails runs testfn with a MockTestingT and asserts that the test fails and the first failure
// logged matches the regexp. The sktest.TestingT passed to testfn is not safe for concurrent use.
func AssertFails(parent sktest.TestingT, regexp string, testfn func(sktest.TestingT)) {
	mock := MockTestingT{}
	wg := sync.WaitGroup{}
	wg.Add(1)
	go func() {
		defer wg.Done()
		testfn(&mock)
	}()
	wg.Wait()
	assert.True(parent, mock.Failed(), "In AssertFails, the test function did not fail.")
	assert.True(parent, len(mock.LogMsgs) > 0, "In AssertFails, the test function did not produce any failure messages.")
	assert.Regexp(parent, regexp, mock.LogMsgs[0])
}
