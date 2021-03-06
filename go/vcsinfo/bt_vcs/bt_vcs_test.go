package bt_vcs

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/mock"
	assert "github.com/stretchr/testify/require"
	"go.skia.org/infra/go/gitiles"
	"go.skia.org/infra/go/gitstore"
	"go.skia.org/infra/go/gitstore/mocks"
	gs_testutils "go.skia.org/infra/go/gitstore/testutils"
	"go.skia.org/infra/go/testutils/unittest"
	"go.skia.org/infra/go/vcsinfo"
	vcs_testutils "go.skia.org/infra/go/vcsinfo/testutils"
)

const (
	skiaRepoURL  = "https://skia.googlesource.com/skia.git"
	localRepoURL = "https://example.com/local.git"
)

func TestVCSSuite(t *testing.T) {
	unittest.LargeTest(t)
	vcs, _, cleanup := setupVCSLocalRepo(t, "master")
	defer cleanup()

	// Run the VCS test suite.
	vcs_testutils.TestDisplay(t, vcs)
	vcs_testutils.TestFrom(t, vcs)
	vcs_testutils.TestByIndex(t, vcs)
	vcs_testutils.TestLastNIndex(t, vcs)
	vcs_testutils.TestRange(t, vcs)
}

func TestBranchInfo(t *testing.T) {
	unittest.LargeTest(t)
	vcs, gitStore, cleanup := setupVCSLocalRepo(t, "")
	defer cleanup()

	ctx := context.TODO()
	branchPointers, err := gitStore.GetBranches(ctx)
	assert.NoError(t, err)
	branches := []string{}
	for branchName := range branchPointers {
		if branchName != "" {
			branches = append(branches, branchName)
		}
	}

	vcs_testutils.TestBranchInfo(t, vcs, branches)
}

// TestGetFile makes sure that we can use gittiles to fetch an
// arbitrary file (DEPS) from the Skia repo at a chosen commit.
func TestGetFile(t *testing.T) {
	unittest.LargeTest(t)
	gtRepo := gitiles.NewRepo(skiaRepoURL, "", nil)
	hash := "9be246ed747fd1b900013dd0596aed0b1a63a1fa"
	vcs := &BigTableVCS{
		repo: gtRepo,
	}
	_, err := vcs.GetFile(context.TODO(), "DEPS", hash)
	assert.NoError(t, err)
}

// TestDetailsCaching makes sure that multiple calls to Details do
// not result in multiple calls to the underlying gitstore, that is,
// the details per commit hash are cached.
func TestDetailsCaching(t *testing.T) {
	unittest.SmallTest(t)

	mg := &mocks.GitStore{}
	defer mg.AssertExpectations(t)

	commits := makeTestLongCommits()

	mg.On("GetBranches", anyContext).Return(makeTestBranchPointerMap(), nil)
	mg.On("RangeN", anyContext, 0, 4, "master").Return(makeTestIndexCommits(), nil)
	startWithEmptyCache(mg)
	mg.On("Get", anyContext, []string{firstHash}).Return([]*vcsinfo.LongCommit{commits[0]}, nil).Once()

	vcs, err := New(mg, "master", nil, nil, disableVCSTracker)
	assert.NoError(t, err)

	// query details 3 times, and make sure it uses the cache after the
	// first time. Since we said Once() on the mocked Get function, we are
	// assured that gitstore.Get() is only called once.
	c, err := vcs.Details(context.TODO(), firstHash, false)
	assert.NoError(t, err)
	assert.Equal(t, commits[0], c)
	assert.Nil(t, c.Branches)
	c, err = vcs.Details(context.TODO(), firstHash, false)
	assert.NoError(t, err)
	assert.Equal(t, commits[0], c)
	c, err = vcs.Details(context.TODO(), firstHash, false)
	assert.NoError(t, err)
	assert.Equal(t, commits[0], c)
}

// TestDetailsBranchInfo tests that the Branches field is filled out when requested.
func TestDetailsBranchInfo(t *testing.T) {
	unittest.SmallTest(t)

	mg := &mocks.GitStore{}
	defer mg.AssertExpectations(t)

	commits := makeTestLongCommits()
	indices := makeTestIndexCommits()

	mg.On("GetBranches", anyContext).Return(makeTestBranchPointerMap(), nil)
	mg.On("RangeN", anyContext, 0, 4, "master").Return(indices, nil)
	startWithFullCache(mg) // full cache, but it won't have branch info
	mg.On("Get", anyContext, []string{firstHash}).Return([]*vcsinfo.LongCommit{commits[0]}, nil)

	mg.On("RangeByTime", anyContext, firstTime, mock.AnythingOfType("time.Time"), "master").Return(
		[]*vcsinfo.IndexCommit{indices[0]}, nil)

	vcs, err := New(mg, "master", nil, nil, disableVCSTracker)
	assert.NoError(t, err)

	c, err := vcs.Details(context.TODO(), firstHash, true)
	assert.NotNil(t, c)
	assert.Equal(t, map[string]bool{
		"master": true,
	}, c.Branches)
}

// TestDetailsBranchInfoCaching validates the cache is cognizant of branch info
func TestDetailsBranchInfoCaching(t *testing.T) {
	unittest.SmallTest(t)

	mg := &mocks.GitStore{}
	defer mg.AssertExpectations(t)

	commits := makeTestLongCommits()
	indices := makeTestIndexCommits()

	mg.On("GetBranches", anyContext).Return(makeTestBranchPointerMap(), nil)
	mg.On("RangeN", anyContext, 0, 4, "master").Return(indices, nil)
	startWithEmptyCache(mg)
	mg.On("Get", anyContext, []string{firstHash}).Return([]*vcsinfo.LongCommit{commits[0]}, nil).Twice()

	mg.On("RangeByTime", anyContext, firstTime, mock.AnythingOfType("time.Time"), "master").Return(
		[]*vcsinfo.IndexCommit{indices[0]}, nil).Once()

	vcs, err := New(mg, "master", nil, nil, disableVCSTracker)
	assert.NoError(t, err)

	// query details 3 times, and make sure it uses the cache after the
	// first two times (cache miss on the second time because we requested)
	// branch info.
	c, err := vcs.Details(context.TODO(), firstHash, false)
	assert.NoError(t, err)
	assert.Nil(t, c.Branches)
	c, err = vcs.Details(context.TODO(), firstHash, true)
	assert.NoError(t, err)
	assert.NotNil(t, c.Branches)
	c, err = vcs.Details(context.TODO(), firstHash, false)
	assert.NoError(t, err)
	// Branches is still here because it was in the cache. This should be fine,
	// to give clients extra information when they didn't necessarily ask for it.
	assert.NotNil(t, c.Branches)
}

// TestDetailsMultiCaching makes sure that multiple calls to DetailsMulti do
// not result in multiple calls to the underlying gitstore, that is,
// the details per commit hash are cached.
func TestDetailsMultiCaching(t *testing.T) {
	unittest.SmallTest(t)

	mg := &mocks.GitStore{}
	defer mg.AssertExpectations(t)

	commits := makeTestLongCommits()

	mg.On("GetBranches", anyContext).Return(makeTestBranchPointerMap(), nil)
	mg.On("RangeN", anyContext, 0, 4, "master").Return(makeTestIndexCommits(), nil)
	startWithEmptyCache(mg)
	mg.On("Get", anyContext, []string{firstHash, secondHash}).Return([]*vcsinfo.LongCommit{commits[0], commits[1]}, nil).Once()

	vcs, err := New(mg, "master", nil, nil, disableVCSTracker)
	assert.NoError(t, err)

	// query details 3 times, and make sure it uses the cache after the
	// first time. Since we said Once() on the mocked Get function, we are
	// assured that gitstore.Get() is only called once.
	c, err := vcs.DetailsMulti(context.TODO(), []string{firstHash, secondHash}, false)
	assert.NoError(t, err)
	assert.NotNil(t, c)
	assert.Len(t, c, 2)
	assert.Equal(t, commits[0], c[0])
	assert.Equal(t, commits[1], c[1])
	c, err = vcs.DetailsMulti(context.TODO(), []string{firstHash, secondHash}, false)
	assert.NoError(t, err)
	assert.NotNil(t, c)
	assert.Len(t, c, 2)
	assert.Equal(t, commits[0], c[0])
	assert.Equal(t, commits[1], c[1])
	c, err = vcs.DetailsMulti(context.TODO(), []string{firstHash, secondHash}, false)
	assert.NoError(t, err)
	assert.NotNil(t, c)
	assert.Len(t, c, 2)
	assert.Equal(t, commits[0], c[0])
	assert.Equal(t, commits[1], c[1])
}

// TestDetailsMultiPartialCaching is like TestDetailsMultiCaching, except that
// it makes sure a partial hit (e.g. some of the hashes are cached) results
// in only the non-cached subset being queried to GitStore.
func TestDetailsMultiPartialCaching(t *testing.T) {
	unittest.SmallTest(t)

	mg := &mocks.GitStore{}
	defer mg.AssertExpectations(t)

	commits := makeTestLongCommits()

	mg.On("GetBranches", anyContext).Return(makeTestBranchPointerMap(), nil)
	mg.On("RangeN", anyContext, 0, 4, "master").Return(makeTestIndexCommits(), nil)
	startWithEmptyCache(mg)
	mg.On("Get", anyContext, []string{firstHash, thirdHash}).Return([]*vcsinfo.LongCommit{commits[0], commits[2]}, nil).Once()
	mg.On("Get", anyContext, []string{secondHash}).Return([]*vcsinfo.LongCommit{commits[1]}, nil).Once()

	vcs, err := New(mg, "master", nil, nil, disableVCSTracker)
	assert.NoError(t, err)

	// query details 3 times, and make sure it uses the cache after the
	// first two times. Since we said Once() on the mocked Get functions, we are
	// assured that gitstore.Get() is only called twice, - once for the first 2 hashes
	// and then for the follow up hash.
	c, err := vcs.DetailsMulti(context.TODO(), []string{firstHash, thirdHash}, false)
	assert.NoError(t, err)
	assert.NotNil(t, c)
	assert.Len(t, c, 2)
	assert.Equal(t, commits[0], c[0])
	assert.Equal(t, commits[2], c[1])
	c, err = vcs.DetailsMulti(context.TODO(), []string{firstHash, secondHash, thirdHash}, false)
	assert.NoError(t, err)
	assert.NotNil(t, c)
	assert.Len(t, c, 3)
	assert.Equal(t, commits[0], c[0])
	assert.Equal(t, commits[1], c[1])
	assert.Equal(t, commits[2], c[2])
	c, err = vcs.DetailsMulti(context.TODO(), []string{firstHash, secondHash, thirdHash}, false)
	assert.NoError(t, err)
	assert.NotNil(t, c)
	assert.Len(t, c, 3)
	assert.Equal(t, commits[0], c[0])
	assert.Equal(t, commits[1], c[1])
	assert.Equal(t, commits[2], c[2])
}

// setupVCSLocalRepo loads the test repo into a new GitStore and returns an instance of vcsinfo.VCS.
func setupVCSLocalRepo(t *testing.T, branch string) (vcsinfo.VCS, gitstore.GitStore, func()) {
	repoDir, cleanup := vcs_testutils.InitTempRepo()
	_, _, btgs := gs_testutils.SetupAndLoadBTGitStore(t, localRepoURL, repoDir, true)
	vcs, err := New(btgs, branch, nil, nil, 0)
	assert.NoError(t, err)
	return vcs, btgs, cleanup
}

func startWithEmptyCache(mg *mocks.GitStore) {
	// In the call to New(), Update fetches all the LongCommits for things in the index
	// commits - we return nil for all of these to keep the cache empty (when testing)
	// the cache logic.
	rv := []*vcsinfo.LongCommit{nil, nil, nil}
	mg.On("Get", anyContext, []string{firstHash, secondHash, thirdHash}).Return(rv, nil).Once()
}

func startWithFullCache(mg *mocks.GitStore) {
	// In the call to New(), Update fetches all the LongCommits for things in the index
	// commits - we return the real data here to make sure the detailsCache is full.
	mg.On("Get", anyContext, []string{firstHash, secondHash, thirdHash}).Return(makeTestLongCommits(), nil).Once()
}

const (
	// arbitrary sha1 hashes
	firstHash  = "ae76331b95dfc399cd776d2fc68021e0db03cc4f"
	secondHash = "b295e0bdde1938d1fbfd343e5a3e569e868e1465"
	thirdHash  = "cf70f4c33de2200b76651bbe1e54aa55fcd77447"

	// For the sake of unit tests, we'd rather the auto-polling functionality
	// of the bt_vcs not be running in the background, which could cause
	// nondeterminism
	disableVCSTracker = 0
)

var (
	// arbitrary times
	firstTime  = time.Date(2019, time.May, 2, 12, 0, 3, 0, time.UTC)
	secondTime = time.Date(2019, time.May, 2, 14, 1, 3, 0, time.UTC)
	thirdTime  = time.Date(2019, time.May, 2, 17, 5, 3, 0, time.UTC)

	anyContext = mock.AnythingOfType("*context.emptyCtx")
)

// This test data (for a repo of 3 commits) is returned via functions
// to make it convenient to have a copy of the data for each test,
// so the tests can write all over the returned values w/o impacting
// tests that follow.
func makeTestLongCommits() []*vcsinfo.LongCommit {
	return []*vcsinfo.LongCommit{
		{
			ShortCommit: &vcsinfo.ShortCommit{
				Author:  "alpha@example.com",
				Hash:    firstHash,
				Subject: "initial commit",
			},
			Body:      "awesome message",
			Parents:   []string{},
			Timestamp: firstTime,
		},
		{
			ShortCommit: &vcsinfo.ShortCommit{
				Author:  "beta@example.com",
				Hash:    secondHash,
				Subject: "followup commit",
			},
			Body:      "bug fixes",
			Parents:   []string{firstHash},
			Timestamp: secondTime,
		},
		{
			ShortCommit: &vcsinfo.ShortCommit{
				Author:  "gamma@example.com",
				Hash:    thirdHash,
				Subject: "last commit",
			},
			Body:      "now deprecated",
			Parents:   []string{secondHash},
			Timestamp: thirdTime,
		},
	}
}

func makeTestIndexCommits() []*vcsinfo.IndexCommit {
	return []*vcsinfo.IndexCommit{
		{
			Hash:      firstHash,
			Index:     0,
			Timestamp: firstTime,
		},
		{
			Hash:      secondHash,
			Index:     1,
			Timestamp: secondTime,
		},
		{
			Hash:      thirdHash,
			Index:     2,
			Timestamp: thirdTime,
		},
	}
}

func makeTestBranchPointerMap() map[string]*gitstore.BranchPointer {
	return map[string]*gitstore.BranchPointer{
		"master": {
			Head:  "master",
			Index: 3,
		},
	}
}
