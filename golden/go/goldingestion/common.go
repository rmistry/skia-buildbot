package goldingestion

import (
	"fmt"
	"io"
	"sort"
	"strings"

	"go.skia.org/infra/go/ingestion"
	"go.skia.org/infra/go/sklog"
	"go.skia.org/infra/go/tiling"
	tracedb "go.skia.org/infra/go/trace/db"
	"go.skia.org/infra/go/util"
	"go.skia.org/infra/golden/go/jsonio"
	"go.skia.org/infra/golden/go/types"
)

// idAndParams constructs the Trace ID and the Trace params from the keys and options.
// It returns the id as a string of all the values, in the alphabetic order of the
// keys, separated by a colon. The trace params returned are a single map of
// key-> values. "Options" are omitted from the trace id, as per design.
func idAndParams(dm *DMResults, r *jsonio.Result) (tiling.TraceId, map[string]string) {
	combinedLen := len(dm.Key) + len(r.Key)
	traceIdParts := make(map[string]string, combinedLen)
	params := make(map[string]string, combinedLen+len(r.Options))
	for k, v := range dm.Key {
		traceIdParts[k] = v
		params[k] = v
	}
	for k, v := range r.Key {
		traceIdParts[k] = v
		params[k] = v
	}
	for k, v := range r.Options {
		params[k] = v
	}

	keys := []string{}
	for k := range traceIdParts {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	values := []string{}
	for _, k := range keys {
		values = append(values, traceIdParts[k])
	}
	return tiling.TraceId(strings.Join(values, ":")), params
}

// extractTraceDBEntries returns the traceDB entries to be inserted into the data store.
func extractTraceDBEntries(dm *DMResults) (map[tiling.TraceId]*tracedb.Entry, error) {
	ret := make(map[tiling.TraceId]*tracedb.Entry, len(dm.Results))
	for _, result := range dm.Results {
		traceId, params := idAndParams(dm, result)
		if ignoreResult(dm, params) {
			continue
		}

		ret[traceId] = &tracedb.Entry{
			Params: params,
			Value:  []byte(result.Digest),
		}
	}

	// If all results were ignored then we return an error.
	if len(ret) == 0 {
		return nil, fmt.Errorf("No valid results in file %s.", dm.name)
	}

	return ret, nil
}

// ignoreResult returns true if the result with the given parameters should be
// ignored.
func ignoreResult(dm *DMResults, params map[string]string) bool {
	// Ignore anything that is not a png.
	if ext, ok := params["ext"]; !ok || (ext != "png") {
		return true
	}

	// Make sure the test name meets basic requirements.
	testName := params[types.PRIMARY_KEY_FIELD]

	// Ignore results that don't have a test given and log an error since that
	// should not happen. But we want to keep other results in the same input file.
	if testName == "" {
		sklog.Errorf("Missing test name in %s", dm.name)
		return true
	}

	// Make sure the test name does not exceed the allowed length.
	if len(testName) > types.MAXIMUM_NAME_LENGTH {
		sklog.Errorf("Received test name which is longer than the allowed %d bytes: %s", types.MAXIMUM_NAME_LENGTH, testName)
		return true
	}

	return false
}

// DMResults enhances GoldResults with fields used for internal processing.
type DMResults struct {
	*jsonio.GoldResults

	// name is the name/path of the file where this came from.
	name string
}

// Name returns the name/path from which these results were parsed.
func (d *DMResults) Name() string {
	return d.name
}

// ParseDMResultsFromReader parses the JSON stream out of the io.ReadCloser
// into a DMResults instance and closes the reader.
func ParseDMResultsFromReader(r io.ReadCloser, name string) (*DMResults, error) {
	defer util.Close(r)

	goldResults, _, err := jsonio.ParseGoldResults(r)
	if err != nil {
		return nil, fmt.Errorf("Failed to decode JSON: %s", err)
	}

	dmResults := &DMResults{GoldResults: goldResults}
	dmResults.name = name
	return dmResults, nil
}

// processDMResults opens the given JSON input file and processes it, converting
// it into a goldingestion.DMResults object and returning it.
func processDMResults(resultsFile ingestion.ResultFileLocation) (*DMResults, error) {
	r, err := resultsFile.Open()
	if err != nil {
		return nil, err
	}

	return ParseDMResultsFromReader(r, resultsFile.Name())
}
