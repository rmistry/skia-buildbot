// Package indexer continuously creates an index of the test results
// as the tiles, expectations and ignores change.
package indexer

import (
	"net/url"
	"sync"
	"time"

	"go.skia.org/infra/go/metrics2"
	"go.skia.org/infra/go/paramtools"
	"go.skia.org/infra/go/skerr"
	"go.skia.org/infra/go/sklog"
	"go.skia.org/infra/go/tiling"
	"go.skia.org/infra/go/vcsinfo"
	"go.skia.org/infra/go/vcsinfo/bt_vcs"
	"go.skia.org/infra/golden/go/blame"
	"go.skia.org/infra/golden/go/diff"
	"go.skia.org/infra/golden/go/digest_counter"
	"go.skia.org/infra/golden/go/digesttools"
	"go.skia.org/infra/golden/go/expstorage"
	"go.skia.org/infra/golden/go/paramsets"
	"go.skia.org/infra/golden/go/pdag"
	"go.skia.org/infra/golden/go/shared"
	"go.skia.org/infra/golden/go/storage"
	"go.skia.org/infra/golden/go/summary"
	"go.skia.org/infra/golden/go/types"
	"go.skia.org/infra/golden/go/warmer"
)

const (
	// Event emitted when the indexer updates the search index.
	// Callback argument: *SearchIndex
	EV_INDEX_UPDATED = "indexer:index-updated"

	// Metric to track the number of digests that do not have be uploaded by bots.
	METRIC_KNOWN_HASHES = "known_digests"
)

// SearchIndex contains everything that is necessary to search
// our current knowledge about test results. It should be
// considered as immutable. Whenever the underlying data changes,
// a new index is calculated via a pdag.
type SearchIndex struct {
	// The indices of these slices are the int values of types.IgnoreState
	dCounters         []digest_counter.DigestCounter
	summaries         []summary.SummaryMap
	paramsetSummaries []paramsets.ParamSummary

	cpxTile types.ComplexTile
	blamer  blame.Blamer
	warmer  warmer.DiffWarmer

	// This is set by the indexing pipeline when we just want to update
	// individual tests that have changed.
	testNames types.TestNameSet
	storages  *storage.Storage
}

// newSearchIndex creates a new instance of SearchIndex. It is not intended to
// be used outside of this package. SearchIndex instances are created by the
// Indexer and retrieved via GetIndex().
func newSearchIndex(storages *storage.Storage, cpxTile types.ComplexTile, w warmer.DiffWarmer) *SearchIndex {
	return &SearchIndex{
		// The indices of these slices are the int values of types.IgnoreState
		dCounters:         make([]digest_counter.DigestCounter, 2),
		summaries:         make([]summary.SummaryMap, 2),
		paramsetSummaries: make([]paramsets.ParamSummary, 2),
		cpxTile:           cpxTile,
		blamer:            nil,
		warmer:            w,
		storages:          storages,
	}
}

// CpxTile returns the current complex tile from which simpler tiles, like one without ignored
// traces can be retrieved
func (idx *SearchIndex) CpxTile() types.ComplexTile {
	return idx.cpxTile
}

// GetIgnoreMatcher returns a matcher for the ignore rules that were used to
// build the tile with ignores.
func (idx *SearchIndex) GetIgnoreMatcher() paramtools.ParamMatcher {
	return idx.cpxTile.IgnoreRules()
}

// Proxy to digest_counter.DigestCounter.ByTest
func (idx *SearchIndex) DigestCountsByTest(is types.IgnoreState) map[types.TestName]digest_counter.DigestCount {
	return idx.dCounters[is].ByTest()
}

// Proxy to digest_counter.DigestCounter.MaxDigestsByTest
func (idx *SearchIndex) MaxDigestsByTest(is types.IgnoreState) map[types.TestName]types.DigestSet {
	return idx.dCounters[is].MaxDigestsByTest()
}

// Proxy to digest_counter.DigestCounter.ByTrace
func (idx *SearchIndex) DigestCountsByTrace(is types.IgnoreState) map[tiling.TraceId]digest_counter.DigestCount {
	return idx.dCounters[is].ByTrace()
}

// ByQuery returns a DigestCount of all the digests that match the given query.
func (idx *SearchIndex) DigestCountsByQuery(query url.Values, is types.IgnoreState) digest_counter.DigestCount {
	return idx.dCounters[is].ByQuery(idx.cpxTile.GetTile(is), query)
}

// Proxy to summary.Summary.Get.
func (idx *SearchIndex) GetSummaries(is types.IgnoreState) summary.SummaryMap {
	return idx.summaries[is]
}

// Proxy to summary.CalcSummaries.
func (idx *SearchIndex) CalcSummaries(testNames types.TestNameSet, query url.Values, is types.IgnoreState, head bool) (summary.SummaryMap, error) {
	dCounter := idx.dCounters[is]
	return summary.NewSummaryMap(idx.storages, idx.cpxTile.GetTile(is), dCounter, idx.blamer, testNames, query, head)
}

// Proxy to paramsets.Get
func (idx *SearchIndex) GetParamsetSummary(test types.TestName, digest types.Digest, is types.IgnoreState) paramtools.ParamSet {
	return idx.paramsetSummaries[is].Get(test, digest)
}

// Proxy to paramsets.GetByTest
func (idx *SearchIndex) GetParamsetSummaryByTest(is types.IgnoreState) map[types.TestName]map[types.Digest]paramtools.ParamSet {
	return idx.paramsetSummaries[is].GetByTest()
}

// Proxy to blame.Blamer.GetBlame.
func (idx *SearchIndex) GetBlame(test types.TestName, digest types.Digest, commits []*tiling.Commit) *blame.BlameDistribution {
	if idx.blamer == nil {
		// should never happen - indexer should have this initialized
		// before the web server starts serving requests.
		return nil
	}
	return idx.blamer.GetBlame(test, digest, commits)
}

// Indexer is the type that drive continously indexing as the underlying
// data change. It uses a DAG that encodes the dependencies of the
// different components of an index and creates a processing pipeline on top
// of it.
type Indexer struct {
	storages          *storage.Storage
	warmer            warmer.DiffWarmer
	pipeline          *pdag.Node
	indexTestsNode    *pdag.Node
	writeBaselineNode *pdag.Node
	lastIndex         *SearchIndex
	mutex             sync.RWMutex
}

// New returns a new Indexer instance. It synchronously indexes the initially
// available tile. If the indexing fails an error is returned.
// The provided interval defines how often the index should be refreshed.
func New(storages *storage.Storage, w warmer.DiffWarmer, interval time.Duration) (*Indexer, error) {
	ret := &Indexer{
		storages: storages,
		warmer:   w,
	}

	// Set up the processing pipeline.
	root := pdag.NewNodeWithParents(pdag.NoOp)

	// At the top level, Add the DigestCounters...
	countsNodeInclude := root.Child(calcDigestCountsInclude)
	// These are run in parallel because they can take tens of seconds
	// in large repos like Skia.
	countsNodeExclude := root.Child(calcDigestCountsExclude)

	// Node that triggers blame and writing baselines.
	// This is used to trigger when expectations change.
	// We don't need to re-calculate DigestCounts if the
	// expectations change because the DigestCounts don't care about
	// the expectations, only on the tile.
	indexTestsNode := root.Child(pdag.NoOp)

	// ... and invoke the Blamer to calculate the blames.
	blamerNode := indexTestsNode.Child(calcBlame)

	// Write baselines whenever a new tile is processed, new commits become available, or
	// when the expectations change.
	writeBaselineNode := indexTestsNode.Child(writeMasterBaseline)

	// Parameters depend on DigestCounter.
	paramsNodeInclude := pdag.NewNodeWithParents(calcParamsetsInclude, countsNodeInclude)
	// These are run in parallel because they can take tens of seconds
	// in large repos like Skia.
	paramsNodeExclude := pdag.NewNodeWithParents(calcParamsetsExclude, countsNodeExclude)

	// Write known hashes after ignores are computed. DigestCount is a
	// convenient way to get all the hashes, so that's what this node uses.
	writeHashes := countsNodeInclude.Child(writeKnownHashesList)

	// Summaries depend on DigestCounter and Blamer.
	summariesNode := pdag.NewNodeWithParents(calcSummaries, countsNodeInclude, countsNodeExclude, blamerNode)

	// The Warmer depends on summaries.
	pdag.NewNodeWithParents(runWarmer, summariesNode)

	// Set the result on the Indexer instance, once summaries, parameters and writing
	// the hash files is done.
	pdag.NewNodeWithParents(ret.setIndex, summariesNode, paramsNodeInclude, paramsNodeExclude, writeHashes)

	ret.pipeline = root
	ret.indexTestsNode = indexTestsNode
	ret.writeBaselineNode = writeBaselineNode

	// Process the first tile and start the indexing process.
	return ret, ret.start(interval)
}

// GetIndex returns the current index, which is updated continuously in the
// background. The returned instances of SearchIndex can be considered immutable
// and is not going to change. It should be used to handle an entire request
// to provide consistent information.
func (ixr *Indexer) GetIndex() *SearchIndex {
	ixr.mutex.RLock()
	defer ixr.mutex.RUnlock()
	return ixr.lastIndex
}

// start builds the initial index and starts the background
// process to continuously build indices.
func (ixr *Indexer) start(interval time.Duration) error {
	if interval == 0 {
		sklog.Warning("Not starting indexer because duration was 0")
		return nil
	}
	defer shared.NewMetricsTimer("initial_synchronous_index").Stop()
	// Build the first index synchronously.
	tileStream := ixr.storages.GetTileStreamNow(interval, "gold-indexer")
	if err := ixr.executePipeline(<-tileStream); err != nil {
		return err
	}

	// When the master expectations change, update the blamer and its dependents. We choose size
	// 100 because that is plenty capture an unlikely torrent of changes (they are usually triggered
	// by a user).
	expCh := make(chan types.Expectations, 100)
	ixr.storages.EventBus.SubscribeAsync(expstorage.EV_EXPSTORAGE_CHANGED, func(e interface{}) {
		// Schedule the list of test names to be recalculated.
		expCh <- e.(*expstorage.EventExpectationChange).TestChanges
	})

	// When the expectations of a Gerrit issue change then trigger pushing the
	// new expectations to GCS.
	ixr.storages.EventBus.SubscribeAsync(expstorage.EV_TRYJOB_EXP_CHANGED, ixr.writeIssueBaseline)

	// When new commits have become available trigger writing the baselines. We choose size 100
	// because that is large enough to handle an unlikely torrent of commits being added to the repo.
	commitCh := make(chan []*vcsinfo.IndexCommit, 100)
	ixr.storages.EventBus.SubscribeAsync(bt_vcs.EV_NEW_GIT_COMMIT, func(e interface{}) {
		commitCh <- e.([]*vcsinfo.IndexCommit)
	})

	// Keep building indices for different types of events. This is the central
	// event loop of the indexer.
	go func() {
		var cpxTile types.ComplexTile
		for {
			testChanges := []types.Expectations{}

			// See if there is a tile or changed tests.
			cpxTile = nil
			select {
			// Catch a new tile.
			case cpxTile = <-tileStream:
				sklog.Infof("Indexer saw a new tile")

				// Catch any test changes.
			case tn := <-expCh:
				testChanges = append(testChanges, tn)
				sklog.Infof("Indexer saw %d tests change", len(tn))

				// Catch changes in the commits.
			case <-commitCh:
				sklog.Infof("Indexer saw commits change")
			}

			// Drain all input channels, effectively bunching signals together that arrive in short
			// succession.
		DrainLoop:
			for {
				select {
				case tn := <-expCh:
					testChanges = append(testChanges, tn)
				case <-commitCh:
				default:
					break DrainLoop
				}
			}

			// If there is a tile, re-index everything and forget the
			// individual tests that changed.
			if cpxTile != nil {
				if err := ixr.executePipeline(cpxTile); err != nil {
					sklog.Errorf("Unable to index tile: %s", err)
				}
			} else if len(testChanges) > 0 {
				// Only index the tests that have changed.
				ixr.indexTests(testChanges)
			} else {
				// At this point new commits have been discovered and we
				// just want to write the baselines.
				ixr.writeBaselines()
			}
		}
	}()

	return nil
}

// executePipeline runs the given tile through the the indexing pipeline.
// pipeline.Trigger blocks until everything is done, so this function will as well.
func (ixr *Indexer) executePipeline(cpxTile types.ComplexTile) error {
	defer shared.NewMetricsTimer("indexer_execute_pipeline").Stop()
	// Create a new index from the given tile.
	return ixr.pipeline.Trigger(newSearchIndex(ixr.storages, cpxTile, ixr.warmer))
}

// writeBaselines triggers the node that causes baselines to be written to GCS.
func (ixr *Indexer) writeBaselines() {
	idx := ixr.cloneLastIndex()
	if err := ixr.writeBaselineNode.Trigger(idx); err != nil {
		sklog.Errorf("Error writing baselines: %s", err)
	}
}

// indexTest creates an updated index by indexing the given list of expectation changes.
func (ixr *Indexer) indexTests(testChanges []types.Expectations) {
	// Get all the test names that had expectations changed.
	testNames := types.TestNameSet{}
	for _, testChange := range testChanges {
		for testName := range testChange {
			testNames[testName] = true
		}
	}
	if len(testNames) == 0 {
		return
	}

	sklog.Infof("Going to re-index %d tests", len(testNames))

	defer shared.NewMetricsTimer("index_tests").Stop()
	newIdx := ixr.cloneLastIndex()
	// Set the testNames such that we only recompute those tests.
	newIdx.testNames = testNames
	if err := ixr.indexTestsNode.Trigger(newIdx); err != nil {
		sklog.Errorf("Error indexing tests: %v \n\n Got error: %s", testNames.Keys(), err)
	}
}

// cloneLastIndex returns a copy of the most recent index.
func (ixr *Indexer) cloneLastIndex() *SearchIndex {
	lastIdx := ixr.GetIndex()
	return &SearchIndex{
		cpxTile:           lastIdx.cpxTile,
		dCounters:         lastIdx.dCounters,         // stay the same even if expectations change.
		paramsetSummaries: lastIdx.paramsetSummaries, // stay the same even if expectations change.

		summaries: []summary.SummaryMap{
			lastIdx.summaries[types.ExcludeIgnoredTraces], // immutable, but may be replaced if
			lastIdx.summaries[types.IncludeIgnoredTraces], // expectations change
		},

		blamer:   nil, // This will need to be recomputed if expectations change.
		warmer:   ixr.warmer,
		storages: lastIdx.storages,

		// Force testNames to be empty, just to be sure we re-compute everything by default
		testNames: nil,
	}
}

// setIndex sets the lastIndex value at the very end of the pipeline.
func (ixr *Indexer) setIndex(state interface{}) error {
	newIndex := state.(*SearchIndex)
	ixr.mutex.Lock()
	defer ixr.mutex.Unlock()
	ixr.lastIndex = newIndex
	if ixr.storages.EventBus != nil {
		ixr.storages.EventBus.Publish(EV_INDEX_UPDATED, state, false)
	}
	return nil
}

// writeIssueBaseline handles changes to baselines for Gerrit issues and dumps
// the updated baseline to disk.
func (ixr *Indexer) writeIssueBaseline(evData interface{}) {
	if !ixr.storages.Baseliner.CanWriteBaseline() {
		return
	}

	issueID := evData.(*expstorage.EventExpectationChange).IssueID
	if issueID <= 0 {
		sklog.Errorf("Invalid issue id received for issue exp change: %d", issueID)
		return
	}

	idx := ixr.GetIndex()
	if err := ixr.storages.Baseliner.PushIssueBaseline(issueID, idx.cpxTile, idx.dCounters[types.ExcludeIgnoredTraces]); err != nil {
		sklog.Errorf("Unable to push baseline for issue %d to GCS: %s", issueID, err)
		return
	}
}

// calcDigestCountsInclude is the pipeline function to calculate DigestCounts from
// the full tile (not applying ignore rules)
func calcDigestCountsInclude(state interface{}) error {
	idx := state.(*SearchIndex)
	is := types.IncludeIgnoredTraces
	idx.dCounters[is] = digest_counter.New(idx.cpxTile.GetTile(is))
	return nil
}

// calcDigestCountsExclude is the pipeline function to calculate DigestCounts from
// the partial tile (applying ignore rules).
func calcDigestCountsExclude(state interface{}) error {
	idx := state.(*SearchIndex)
	is := types.ExcludeIgnoredTraces
	idx.dCounters[is] = digest_counter.New(idx.cpxTile.GetTile(is))
	return nil
}

// calcSummaries is the pipeline function to calculate the summaries.
func calcSummaries(state interface{}) error {
	idx := state.(*SearchIndex)
	for _, is := range types.IgnoreStates {
		dCounter := idx.dCounters[is]
		sum, err := summary.NewSummaryMap(idx.storages, idx.cpxTile.GetTile(is), dCounter, idx.blamer, idx.testNames, nil, true)
		if err != nil {
			return skerr.Fmt("Could not calculate summaries with ignore state %d: %s", is, err)
		}
		if len(idx.testNames) > 0 && idx.summaries[is] != nil {
			idx.summaries[is] = idx.summaries[is].Combine(sum)
		} else {
			idx.summaries[is] = sum
		}
	}

	return nil
}

// calcParamsetsInclude is the pipeline function to calculate the parameters from
// the full tile (not applying ignore rules)
func calcParamsetsInclude(state interface{}) error {
	idx := state.(*SearchIndex)
	is := types.IncludeIgnoredTraces
	idx.paramsetSummaries[is] = paramsets.NewParamSummary(idx.cpxTile.GetTile(is), idx.dCounters[is])
	return nil
}

// calcParamsetsExclude is the pipeline function to calculate the parameters from
// the partial tile (applying ignore rules)
func calcParamsetsExclude(state interface{}) error {
	idx := state.(*SearchIndex)
	is := types.ExcludeIgnoredTraces
	idx.paramsetSummaries[is] = paramsets.NewParamSummary(idx.cpxTile.GetTile(is), idx.dCounters[is])
	return nil
}

// calcBlame is the pipeline function to calculate the blame.
func calcBlame(state interface{}) error {
	idx := state.(*SearchIndex)
	exp, err := idx.storages.ExpectationsStore.Get()
	if err != nil {
		return skerr.Fmt("Could not fetch expectaions needed to calculate blame: %s", err)
	}
	b, err := blame.New(idx.cpxTile.GetTile(types.ExcludeIgnoredTraces), exp)
	if err != nil {
		idx.blamer = nil
		return skerr.Fmt("Could not calculate blame: %s", err)
	}
	idx.blamer = b
	return nil
}

func writeKnownHashesList(state interface{}) error {
	idx := state.(*SearchIndex)

	// Only write the hash file if a storage client is available.
	if idx.storages.GCSClient == nil {
		return nil
	}

	// Trigger writing the hashes list.
	go func() {
		byTest := idx.DigestCountsByTest(types.IncludeIgnoredTraces)
		unavailableDigests := idx.storages.DiffStore.UnavailableDigests()
		// Collect all hashes in the tile that haven't been marked as unavailable yet.
		hashes := types.DigestSet{}
		for _, test := range byTest {
			for k := range test {
				if _, ok := unavailableDigests[k]; !ok {
					hashes[k] = true
				}
			}
		}

		// Make sure they all fetched already. This will block until all digests
		// are on disk or have failed to load repeatedly.
		idx.storages.DiffStore.WarmDigests(diff.PRIORITY_NOW, hashes.Keys(), true)
		unavailableDigests = idx.storages.DiffStore.UnavailableDigests()
		for h := range hashes {
			if _, ok := unavailableDigests[h]; ok {
				delete(hashes, h)
			}
		}

		// Keep track of the number of known hashes since this directly affects how
		// many images the bots have to upload.
		metrics2.GetInt64Metric(METRIC_KNOWN_HASHES).Update(int64(len(hashes)))
		if err := idx.storages.GCSClient.WriteKnownDigests(hashes.Keys()); err != nil {
			sklog.Errorf("Error writing known digests list: %s", err)
		}
	}()
	return nil
}

// writeMasterBaseline asynchronously writes the master baseline to GCS.
func writeMasterBaseline(state interface{}) error {
	idx := state.(*SearchIndex)

	if !idx.storages.Baseliner.CanWriteBaseline() {
		sklog.Warning("Indexer tried to write baselines, but was not allowed to")
		return nil
	}

	// Write the baseline asynchronously.
	// TODO(kjlubick): Does this being asynchronous cause problems?
	go func() {
		if _, err := idx.storages.Baseliner.PushMasterBaselines(idx.cpxTile, ""); err != nil {
			sklog.Errorf("Error pushing master baseline to GCS: %s", err)
		}
	}()

	return nil
}

// runWarmer is the pipeline function to run the warmer. It runs
// asynchronously since its results are not relevant for the searchIndex.
func runWarmer(state interface{}) error {
	idx := state.(*SearchIndex)

	// TODO(kjlubick): Instead of warming everything we should warm non-ignored
	// traces with higher priority.

	is := types.IncludeIgnoredTraces
	exp, err := idx.storages.ExpectationsStore.Get()
	if err != nil {
		return skerr.Fmt("Could not run warmer - expectations failure: %s", err)
	}
	d := digesttools.NewClosestDiffFinder(exp, idx.dCounters[is], idx.storages.DiffStore)

	go idx.warmer.PrecomputeDiffs(idx.summaries[is], idx.testNames, idx.dCounters[is], d)
	return nil
}
