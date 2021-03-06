package blame

import (
	"testing"

	assert "github.com/stretchr/testify/require"
	"go.skia.org/infra/go/testutils/unittest"
	three_devices "go.skia.org/infra/golden/go/testutils/data_three_devices"
)

func TestBlamerGetBlamesForTestThreeDevices(t *testing.T) {
	unittest.SmallTest(t)

	blamer := blamerWithCalculate(t)

	bd := blamer.GetBlamesForTest(three_devices.AlphaTest)
	assert.Len(t, bd, 1)
	b := bd[0]
	assert.NotNil(t, b)

	// The AlphaTest becomes untriaged in exactly the third commit for exactly
	// one trace, so blame is able to identify that author as the one
	// and only culprit.
	assert.Equal(t, WeightedBlame{
		Author: three_devices.ThirdCommitAuthor,
		Prob:   1,
	}, *b)

	bd = blamer.GetBlamesForTest(three_devices.BetaTest)
	assert.Len(t, bd, 1)
	b = bd[0]
	assert.NotNil(t, b)

	// The BetaTest has an untriaged digest in the first commit and is missing
	// data after that, so this is the best that blamer can do.
	assert.Equal(t, WeightedBlame{
		Author: three_devices.FirstCommitAuthor,
		Prob:   1,
	}, *b)

	bd = blamer.GetBlamesForTest("test_that_does_not_exist")
	assert.Len(t, bd, 0)
}

func TestBlamerGetBlameThreeDevices(t *testing.T) {
	unittest.SmallTest(t)

	blamer := blamerWithCalculate(t)
	commits := three_devices.MakeTestCommits()

	// In the first two commits, this untriaged image doesn't show up
	// so GetBlame should return empty.
	bd := blamer.GetBlame(three_devices.AlphaTest, three_devices.AlphaUntriaged1Digest, commits[0:2])
	assert.NotNil(t, bd)
	assert.Equal(t, BlameDistribution{
		Freq: []int{},
		Old:  false,
	}, *bd)

	// Searching in the whole range should indicates that
	// the commit with index 2 (i.e. the third and last one)
	// has the blame
	bd = blamer.GetBlame(three_devices.AlphaTest, three_devices.AlphaUntriaged1Digest, commits[0:3])
	assert.NotNil(t, bd)
	assert.Equal(t, BlameDistribution{
		Freq: []int{2},
		Old:  false,
	}, *bd)

	// The BetaUntriaged1Digest only shows up in the first commit (index 0)
	bd = blamer.GetBlame(three_devices.BetaTest, three_devices.BetaUntriaged1Digest, commits[0:3])
	assert.NotNil(t, bd)
	assert.Equal(t, BlameDistribution{
		Freq: []int{0},
		Old:  false,
	}, *bd)

	// Good digests have no blame ever
	bd = blamer.GetBlame(three_devices.BetaTest, three_devices.BetaGood1Digest, commits[0:3])
	assert.NotNil(t, bd)
	assert.Equal(t, BlameDistribution{
		Freq: []int{},
		Old:  false,
	}, *bd)

	// Negative digests have no blame ever
	bd = blamer.GetBlame(three_devices.AlphaTest, three_devices.AlphaBad1Digest, commits[0:3])
	assert.NotNil(t, bd)
	assert.Equal(t, BlameDistribution{
		Freq: []int{},
		Old:  false,
	}, *bd)
}

// Returns a Blamer filled out with the data from three_devices.
func blamerWithCalculate(t *testing.T) Blamer {
	exp := three_devices.MakeTestExpectations()

	blamer, err := New(three_devices.MakeTestTile(), exp)
	assert.NoError(t, err)

	return blamer
}

const (
	// Directory with testdata.
	TEST_DATA_DIR = "./testdata"

	// Local file location of the test data.
	TEST_DATA_PATH = TEST_DATA_DIR + "/goldentile.json.zip"

	// Folder in the testdata bucket. See go/testutils for details.
	TEST_DATA_STORAGE_PATH = "gold-testdata/goldentile.json.gz"
)

// TestBlamerWithSyntheticData tests a lot of the functionality of
// blamer. TODO(kjlubick) I think it tests too much and should
// be broken up into smaller pieces. Additionally, explaining why
// the data ends up the way it does would aid in readability.
// func TestBlamerWithSyntheticData(t *testing.T) {
// 	unittest.SmallTest(t)
// 	start := time.Now().Unix()
// 	commits := []*tiling.Commit{
// 		{CommitTime: start + 10, Hash: "h1", Author: "John Doe 1"},
// 		{CommitTime: start + 20, Hash: "h2", Author: "John Doe 2"},
// 		{CommitTime: start + 30, Hash: "h3", Author: "John Doe 3"},
// 		{CommitTime: start + 40, Hash: "h4", Author: "John Doe 4"},
// 		{CommitTime: start + 50, Hash: "h5", Author: "John Doe 5"},
// 	}

// 	params := []map[string]string{
// 		{types.PRIMARY_KEY_FIELD: "foo", "config": "8888", types.CORPUS_FIELD: "gm"},
// 		{types.PRIMARY_KEY_FIELD: "foo", "config": "565", types.CORPUS_FIELD: "gm"},
// 		{types.PRIMARY_KEY_FIELD: "foo", "config": "gpu", types.CORPUS_FIELD: "gm"},
// 		{types.PRIMARY_KEY_FIELD: "bar", "config": "8888", types.CORPUS_FIELD: "gm"},
// 		{types.PRIMARY_KEY_FIELD: "bar", "config": "565", types.CORPUS_FIELD: "gm"},
// 		{types.PRIMARY_KEY_FIELD: "bar", "config": "gpu", types.CORPUS_FIELD: "gm"},
// 		{types.PRIMARY_KEY_FIELD: "baz", "config": "565", types.CORPUS_FIELD: "gm"},
// 		{types.PRIMARY_KEY_FIELD: "baz", "config": "gpu", types.CORPUS_FIELD: "gm"},
// 	}

// 	DI_1, DI_2, DI_3 := "digest1", "digest2", "digest3"
// 	DI_4, DI_5, DI_6 := "digest4", "digest5", "digest6"
// 	DI_7, DI_8, DI_9 := "digest7", "digest8", "digest9"
// 	MISS := types.MISSING_DIGEST

// 	digests := [][]string{
// 		{MISS, MISS, DI_1, MISS, MISS},
// 		{MISS, DI_1, DI_1, DI_2, MISS},
// 		{DI_3, MISS, MISS, MISS, MISS},
// 		{DI_5, DI_4, DI_5, DI_5, DI_5},
// 		{DI_6, MISS, DI_4, MISS, MISS},
// 		{MISS, MISS, MISS, MISS, MISS},
// 		{DI_7, DI_7, MISS, DI_8, MISS},
// 		{DI_7, MISS, DI_7, DI_8, MISS},
// 	}

// 	// Make sure the data are consistent and create a mock TileStore.
// 	assert.Equal(t, len(commits), len(digests[0]))
// 	assert.Equal(t, len(digests), len(params))

// 	eventBus := eventbus.New()
// 	storages := &storage.Storage{
// 		ExpectationsStore: expstorage.NewMemExpectationsStore(eventBus),
// 		MasterTileBuilder: mocks.NewMockTileBuilder(t, digests, params, commits),
// 		EventBus:          eventBus,
// 	}
// 	cpxTile, err := storages.GetLastTileTrimmed()

// 	exp, err := storages.ExpectationsStore.Get()
// 	assert.NoError(t, err)
// 	blamer, err := New(cpxTile.GetTile(false), exp)
// 	assert.NoError(t, err)

// 	storages.EventBus.SubscribeAsync(expstorage.EV_EXPSTORAGE_CHANGED, func(e interface{}) {
// 		if err := blamer.Calculate(cpxTile.GetTile(false)); err != nil {
// 			assert.Fail(t, "Async calculate failed")
// 		}
// 	})

// 	// Check when completely untriaged
// 	blameLists, _ := blamer.GetAllBlameLists()
// 	assert.NotNil(t, blameLists)

// 	assert.Equal(t, 3, len(blameLists))
// 	assert.Equal(t, 3, len(blameLists["foo"]))
// 	assert.Equal(t, []int{1, 0, 0, 0}, blameLists["foo"][DI_1].Freq)
// 	assert.Equal(t, []int{1, 0}, blameLists["foo"][DI_2].Freq)
// 	assert.Equal(t, []int{1, 0, 0, 0, 0}, blameLists["foo"][DI_3].Freq)

// 	assert.Equal(t, 3, len(blameLists["bar"]))
// 	assert.Equal(t, []int{2, 0, 0, 0}, blameLists["bar"][DI_4].Freq)
// 	assert.Equal(t, []int{1, 0, 0, 0, 0}, blameLists["bar"][DI_5].Freq)
// 	assert.Equal(t, []int{1, 0, 0, 0, 0}, blameLists["bar"][DI_6].Freq)

// 	assert.Equal(t, &BlameDistribution{Freq: []int{1}}, blamer.GetBlame("foo", DI_1, commits))
// 	assert.Equal(t, &BlameDistribution{Freq: []int{3}}, blamer.GetBlame("foo", DI_2, commits))
// 	assert.Equal(t, &BlameDistribution{Freq: []int{0}}, blamer.GetBlame("foo", DI_3, commits))
// 	assert.Equal(t, &BlameDistribution{Freq: []int{1}}, blamer.GetBlame("bar", DI_4, commits))
// 	assert.Equal(t, &BlameDistribution{Freq: []int{0}}, blamer.GetBlame("bar", DI_5, commits))
// 	assert.Equal(t, &BlameDistribution{Freq: []int{0}}, blamer.GetBlame("bar", DI_6, commits))

// 	// Classify some digests and re-calculate.
// 	changes := types.Expectations{
// 		"foo": map[string]types.Label{DI_1: types.POSITIVE, DI_2: types.NEGATIVE},
// 		"bar": map[string]types.Label{DI_4: types.POSITIVE, DI_6: types.NEGATIVE},
// 	}
// 	assert.NoError(t, storages.ExpectationsStore.AddChange(changes, ""))

// 	// Wait for the change to propagate.
// 	waitForChange(t, blamer, blameLists)
// 	blameLists, _ = blamer.GetAllBlameLists()

// 	assert.Equal(t, 3, len(blameLists))
// 	assert.Equal(t, 1, len(blameLists["foo"]))
// 	assert.Equal(t, []int{1, 0, 0, 0, 0}, blameLists["foo"][DI_3].Freq)

// 	assert.Equal(t, 1, len(blameLists["bar"]))
// 	assert.Equal(t, []int{1, 0, 0, 0, 0}, blameLists["bar"][DI_5].Freq)
// 	assert.Equal(t, []int{1, 2, 0}, blameLists["baz"][DI_8].Freq)

// 	assert.Equal(t, &BlameDistribution{Freq: []int{0}}, blamer.GetBlame("foo", DI_3, commits))
// 	assert.Equal(t, &BlameDistribution{Freq: []int{0}}, blamer.GetBlame("bar", DI_5, commits))
// 	assert.Equal(t, &BlameDistribution{Freq: []int{3}}, blamer.GetBlame("baz", DI_8, commits))

// 	// Change the underlying tile and trigger with another change.
// 	tile := storages.MasterTileBuilder.GetTile()

// 	// Get the trace for the last parameters and set a value.
// 	gTrace := tile.Traces[mocks.TraceKey(params[5])].(*types.GoldenTrace)
// 	gTrace.Digests[2] = DI_9

// 	assert.NoError(t, storages.ExpectationsStore.AddChange(changes, ""))

// 	// Wait for the change to propagate.
// 	waitForChange(t, blamer, blameLists)
// 	blameLists, _ = blamer.GetAllBlameLists()

// 	assert.Equal(t, 3, len(blameLists))
// 	assert.Equal(t, 1, len(blameLists["foo"]))
// 	assert.Equal(t, 2, len(blameLists["bar"]))
// 	assert.Equal(t, []int{1, 0, 0}, blameLists["bar"][DI_9].Freq)

// 	assert.Equal(t, &BlameDistribution{Freq: []int{2}}, blamer.GetBlame("bar", DI_9, commits))

// 	// Simulate the case where the digest is not found in digest store.
// 	assert.NoError(t, storages.ExpectationsStore.AddChange(changes, ""))
// 	time.Sleep(10 * time.Millisecond)
// 	blameLists, _ = blamer.GetAllBlameLists()
// 	assert.Equal(t, 3, len(blameLists))
// 	assert.Equal(t, 1, len(blameLists["foo"]))
// 	assert.Equal(t, 2, len(blameLists["bar"]))
// 	assert.Equal(t, []int{1, 0, 0}, blameLists["bar"][DI_9].Freq)

// 	assert.Equal(t, &BlameDistribution{Freq: []int{2}}, blamer.GetBlame("bar", DI_9, commits))
// 	assert.Equal(t, &BlameDistribution{Freq: []int{1}}, blamer.GetBlame("bar", DI_9, commits[1:4]))
// 	assert.Equal(t, &BlameDistribution{Freq: []int{}}, blamer.GetBlame("bar", DI_9, commits[0:2]))
// }

// These tests are not ideal in that they depend on blamer being stateful and not
// a stateless helper. TODO(kjlubick): replace them with some functionality
// that tests similar behavior.

// func BenchmarkBlamer(b *testing.B) {
// 	ctx := context.Background()
// 	tileBuilder := mocks.GetTileBuilderFromEnv(b, ctx)

// 	// Get a tile to make sure it's cached.
// 	tileBuilder.GetTile()
// 	b.ResetTimer()
// 	testBlamerWithLiveData(b, tileBuilder)
// }

// func TestBlamerWithLiveData(t *testing.T) {
// 	unittest.LargeTest(t)

// 	err := gcs_testutils.DownloadTestDataFile(t, gcs_testutils.TEST_DATA_BUCKET, TEST_DATA_STORAGE_PATH, TEST_DATA_PATH)
// 	assert.NoError(t, err, "Unable to download testdata.")
// 	defer testutils.RemoveAll(t, TEST_DATA_DIR)

// 	tileStore := mocks.NewMockTileBuilderFromJson(t, TEST_DATA_PATH)
// 	testBlamerWithLiveData(t, tileStore)
// }

// func testBlamerWithLiveData(t assert.TestingT, tileBuilder tracedb.MasterTileBuilder) {
// 	eventBus := eventbus.New()
// 	storages := &storage.Storage{
// 		ExpectationsStore: expstorage.NewMemExpectationsStore(eventBus),
// 		MasterTileBuilder: tileBuilder,
// 		EventBus:          eventBus,
// 	}

// 	blamer := New(storages)
// 	cpxTile, err := storages.GetLastTileTrimmed()
// 	assert.NoError(t, err)
// 	err = blamer.Calculate(cpxTile.GetTile(false))
// 	assert.NoError(t, err)

// 	storages.EventBus.SubscribeAsync(expstorage.EV_EXPSTORAGE_CHANGED, func(e interface{}) {
// 		if err := blamer.Calculate(cpxTile.GetTile(false)); err != nil {
// 			assert.Fail(t, "Async calculate failed")
// 		}
// 	})

// 	// Wait until we have a blamelist.
// 	var blameLists map[string]map[string]*BlameDistribution
// 	for {
// 		blameLists, _ = blamer.GetAllBlameLists()
// 		if blameLists != nil {
// 			break
// 		}
// 	}

// 	tile := storages.MasterTileBuilder.GetTile()

// 	// Since we set the 'First' timestamp of all digest info entries
// 	// to Now. We should get a non-empty blamelist of all digests.
// 	oneTestName := ""
// 	oneDigest := ""
// 	forEachTestDigestDo(tile, func(testName, digest string) {
// 		assert.NotNil(t, blameLists[testName][digest])
// 		assert.True(t, len(blameLists[testName][digest].Freq) > 0)

// 		// Remember the last one for later.
// 		oneTestName, oneDigest = testName, digest
// 	})

// 	// Change the classification of one test and trigger the recalculation.
// 	changes := types.Expectations{
// 		oneTestName: map[string]types.Label{oneDigest: types.POSITIVE},
// 	}
// 	assert.NoError(t, storages.ExpectationsStore.AddChange(changes, ""))

// 	// Wait for change to propagate.
// 	waitForChange(t, blamer, blameLists)
// 	blameLists, _ = blamer.GetAllBlameLists()

// 	// Assert the correctness of the blamelists.
// 	forEachTestDigestDo(tile, func(testName, digest string) {
// 		if (testName == oneTestName) && (digest == oneDigest) {
// 			assert.Nil(t, blameLists[testName][digest])
// 		} else {
// 			assert.NotNil(t, blameLists[testName][digest])
// 			assert.True(t, len(blameLists[testName][digest].Freq) > 0)
// 		}
// 	})

// 	blameLists, _ = blamer.GetAllBlameLists()
// 	// Randomly assign labels to the different digests and make sure
// 	// that the blamelists are correct.
// 	changes = types.Expectations{}
// 	choices := []types.Label{types.POSITIVE, types.NEGATIVE, types.UNTRIAGED}
// 	forEachTestDigestDo(tile, func(testName, digest string) {
// 		// Randomly skip some digests.
// 		label := choices[rand.Int()%len(choices)]
// 		if label != types.UNTRIAGED {
// 			changes.AddDigest(testName, digest, label)
// 		}
// 	})

// 	// Add the labels and wait for the recalculation.
// 	assert.NoError(t, storages.ExpectationsStore.AddChange(changes, ""))
// 	waitForChange(t, blamer, blameLists)
// 	blameLists, commits := blamer.GetAllBlameLists()

// 	expectations, err := storages.ExpectationsStore.Get()
// 	assert.NoError(t, err)

// 	// Verify that the results are plausible.
// 	forEachTestTraceDo(tile, func(testName string, values []string) {
// 		for idx, digest := range values {
// 			if digest != types.MISSING_DIGEST {
// 				label := expectations.Classification(testName, digest)
// 				if label == types.UNTRIAGED {
// 					bl := blameLists[testName][digest]
// 					assert.NotNil(t, bl)
// 					freq := bl.Freq
// 					assert.True(t, len(freq) > 0)
// 					startIdx := len(commits) - len(freq)
// 					assert.True(t, (startIdx >= 0) && (startIdx <= idx), fmt.Sprintf("Expected (%s): Smaller than %d but got %d.", digest, startIdx, idx))
// 				}
// 			}
// 		}
// 	})
// }

// func waitForChange(t assert.TestingT, blamer Blamer, oldBlameLists map[string]map[string]*BlameDistribution) {
// 	assert.NoError(t, testutils.EventuallyConsistent(time.Second*10, func() error {
// 		blameLists, _ := blamer.GetAllBlameLists()
// 		if !reflect.DeepEqual(blameLists, oldBlameLists) {
// 			return nil
// 		}
// 		return testutils.TryAgainErr
// 	}))
// }

// func forEachTestDigestDo(tile *tiling.Tile, fn func(string, string)) {
// 	for _, trace := range tile.Traces {
// 		gTrace := trace.(*types.GoldenTrace)
// 		testName := gTrace.Params()[types.PRIMARY_KEY_FIELD]
// 		for _, digest := range gTrace.Digests {
// 			if digest != types.MISSING_DIGEST {
// 				fn(testName, digest)
// 			}
// 		}
// 	}
// }

// func forEachTestTraceDo(tile *tiling.Tile, fn func(string, []string)) {
// 	tileLen := tile.LastCommitIndex() + 1
// 	for _, trace := range tile.Traces {
// 		gTrace := trace.(*types.GoldenTrace)
// 		testName := gTrace.Params()[types.PRIMARY_KEY_FIELD]
// 		fn(testName, gTrace.Digests[:tileLen])
// 	}
// }
