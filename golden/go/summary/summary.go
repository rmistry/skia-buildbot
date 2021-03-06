// summary summarizes the current state of triaging.
package summary

import (
	"fmt"
	"net/url"
	"sort"
	"sync"

	"go.skia.org/infra/go/sklog"
	"go.skia.org/infra/go/tiling"
	"go.skia.org/infra/golden/go/blame"
	"go.skia.org/infra/golden/go/diff"
	"go.skia.org/infra/golden/go/digest_counter"
	"go.skia.org/infra/golden/go/shared"
	"go.skia.org/infra/golden/go/storage"
	"go.skia.org/infra/golden/go/types"
)

type SummaryMap map[types.TestName]*Summary

// Summary contains rolled up metrics for one test.
// It is immutable and should be thread safe.
type Summary struct {
	Name      types.TestName         `json:"name"`
	Diameter  int                    `json:"diameter"`
	Pos       int                    `json:"pos"`
	Neg       int                    `json:"neg"`
	Untriaged int                    `json:"untriaged"`
	UntHashes types.DigestSlice      `json:"untHashes"`
	Num       int                    `json:"num"`
	Corpus    string                 `json:"corpus"`
	Blame     []*blame.WeightedBlame `json:"blame"`
}

// summaries is a helper struct for calculating SummaryMap.
type summaries struct {
	storages *storage.Storage
	dCounter digest_counter.DigestCounter
	blamer   blame.Blamer
}

// New creates a new instance of Summaries.
func NewSummaryMap(storages *storage.Storage, tile *tiling.Tile, dCounter digest_counter.DigestCounter, blamer blame.Blamer, testNames types.TestNameSet, query url.Values, head bool) (SummaryMap, error) {
	s := summaries{
		storages: storages,
		dCounter: dCounter,
		blamer:   blamer,
	}
	return s.calcSummaries(tile, testNames, query, head)
}

// Combine creates a new SummaryMap from this and the passed
// in map. The passed in map will "win" in the event there are tests
// in both.
func (s SummaryMap) Combine(other SummaryMap) SummaryMap {
	copied := make(SummaryMap, len(s))
	for k, v := range s {
		copied[k] = v
	}

	for k, v := range other {
		copied[k] = v
	}
	return copied
}

// tracePair is used to hold traces, along with their ids.
type tracePair struct {
	id tiling.TraceId
	tr tiling.Trace
}

// calcSummaries returns a Summary of the given tile. If testNames is not empty,
// then restrict the results to only tests with those names. If query is not empty,
// it will be used as an additional filter. Finally, if head is true, only consider
// the single most recent digest per trace.
func (s *summaries) calcSummaries(tile *tiling.Tile, testNames types.TestNameSet, query url.Values, head bool) (SummaryMap, error) {
	defer shared.NewMetricsTimer("calc_summaries_total").Stop()
	sklog.Infof("CalcSummaries: head %v", head)

	ret := SummaryMap{}

	t := shared.NewMetricsTimer("calc_summaries_expectations")
	e, err := s.storages.ExpectationsStore.Get()
	t.Stop()
	if err != nil {
		return nil, fmt.Errorf("Couldn't get expectations: %s", err)
	}

	// Filter down to just the traces we are interested in, based on query.
	filtered := map[types.TestName][]*tracePair{}
	t = shared.NewMetricsTimer("calc_summaries_filter_traces")
	for id, tr := range tile.Traces {
		name := types.TestName(tr.Params()[types.PRIMARY_KEY_FIELD])
		if len(testNames) > 0 && !testNames[name] {
			continue
		}
		if tiling.Matches(tr, query) {
			if slice, ok := filtered[name]; ok {
				filtered[name] = append(slice, &tracePair{tr: tr, id: id})
			} else {
				filtered[name] = []*tracePair{{tr: tr, id: id}}
			}
		}
	}
	t.Stop()

	digestsByTrace := s.dCounter.ByTrace()

	// Now create summaries for each test using the filtered set of traces.
	t = shared.NewMetricsTimer("calc_summaries_tally")
	lastCommitIndex := tile.LastCommitIndex()
	for name, traces := range filtered {
		digestMap := types.DigestSet{}
		// TODO(kjlubick): I don't think corpus is being calculated correctly.
		// It saves whatever the last corpus seen was to the Summary field, but
		// if a tile has a mix of corpora, it looks like it will just summarize
		// everything?
		corpus := ""
		for _, trid := range traces {
			corpus = trid.tr.Params()[types.CORPUS_FIELD]
			if head {
				// Find the last non-missing value in the trace.
				for i := lastCommitIndex; i >= 0; i-- {
					if trid.tr.IsMissing(i) {
						continue
					} else {
						digestMap[trid.tr.(*types.GoldenTrace).Digests[i]] = true
						break
					}
				}
			} else {
				// Use the digestsByTrace if available, otherwise just inspect the trace.
				if t, ok := digestsByTrace[trid.id]; ok {
					for k := range t {
						digestMap[k] = true
					}
				} else {
					for i := lastCommitIndex; i >= 0; i-- {
						if !trid.tr.IsMissing(i) {
							digestMap[trid.tr.(*types.GoldenTrace).Digests[i]] = true
						}
					}
				}
			}
		}
		ret[name] = s.makeSummary(name, e, s.storages.DiffStore, corpus, digestMap.Keys())
	}
	t.Stop()

	return ret, nil
}

// DigestInfo is test name and a digest found in that test. Returned from Search.
type DigestInfo struct {
	Test   types.TestName `json:"test"`
	Digest types.Digest   `json:"digest"`
}

// makeSummary returns a Summary for the given digests.
func (s *summaries) makeSummary(name types.TestName, exp types.Expectations, diffStore diff.DiffStore, corpus string, digests types.DigestSlice) *Summary {
	pos := 0
	neg := 0
	unt := 0
	diamDigests := types.DigestSlice{}
	untHashes := types.DigestSlice{}
	if expectations, ok := exp[name]; ok {
		for _, digest := range digests {
			if dtype, ok := expectations[digest]; ok {
				switch dtype {
				case types.UNTRIAGED:
					unt += 1
					diamDigests = append(diamDigests, digest)
					untHashes = append(untHashes, digest)
				case types.NEGATIVE:
					neg += 1
				case types.POSITIVE:
					pos += 1
					diamDigests = append(diamDigests, digest)
				}
			} else {
				unt += 1
				diamDigests = append(diamDigests, digest)
				untHashes = append(untHashes, digest)
			}
		}
	} else {
		unt += len(digests)
		diamDigests = digests
		untHashes = digests
	}
	sort.Sort(diamDigests)
	sort.Sort(untHashes)
	return &Summary{
		Name: name,
		// TODO(jcgregorio) Make diameter faster, and also make the actual diameter
		// metric better. Until then disable it.  Diameter:  diameter(diamDigests,
		// diffStore),
		Diameter:  0,
		Pos:       pos,
		Neg:       neg,
		Untriaged: unt,
		UntHashes: untHashes,
		Num:       pos + neg + unt,
		Corpus:    corpus,
		Blame:     s.blamer.GetBlamesForTest(name),
	}
}

func diameter(digests types.DigestSlice, diffStore diff.DiffStore) int {
	// TODO Parallelize.
	lock := sync.Mutex{}
	max := 0
	wg := sync.WaitGroup{}
	for {
		if len(digests) <= 2 {
			break
		}
		wg.Add(1)
		go func(d1 types.Digest, d2 types.DigestSlice) {
			defer wg.Done()
			dms, err := diffStore.Get(diff.PRIORITY_NOW, d1, d2)
			if err != nil {
				sklog.Errorf("Unable to get diff: %s", err)
				return
			}
			localMax := 0
			for _, dm := range dms {
				diffMetrics := dm.(*diff.DiffMetrics)
				if diffMetrics.NumDiffPixels > localMax {
					localMax = diffMetrics.NumDiffPixels
				}
			}
			lock.Lock()
			defer lock.Unlock()
			if localMax > max {
				max = localMax
			}
		}(digests[0], digests[1:2])
		digests = digests[1:]
	}
	wg.Wait()
	return max
}
