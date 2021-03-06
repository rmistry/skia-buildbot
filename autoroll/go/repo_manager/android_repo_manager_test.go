package repo_manager

import (
	"context"
	"fmt"
	"io/ioutil"
	"net/http"
	"strings"
	"testing"

	assert "github.com/stretchr/testify/require"
	"go.skia.org/infra/autoroll/go/codereview"
	"go.skia.org/infra/autoroll/go/strategy"
	"go.skia.org/infra/go/exec"
	"go.skia.org/infra/go/gerrit"
	gerrit_mocks "go.skia.org/infra/go/gerrit/mocks"
	"go.skia.org/infra/go/testutils"
	"go.skia.org/infra/go/testutils/unittest"
)

const (
	androidIssueNum        = int64(12345)
	numAndroidChildCommits = 10
)

var (
	androidEmails = []string{"reviewer@chromium.org"}
	childCommits  = []string{
		"5678888888888888888888888888888888888888",
		"1234444444444444444444444444444444444444"}
)

func androidGerrit(t *testing.T, g gerrit.GerritInterface) codereview.CodeReview {
	rv, err := (&codereview.GerritConfig{
		URL:     "https://googleplex-android-review.googlesource.com",
		Project: "platform/external/skia",
		Config:  codereview.GERRIT_CONFIG_ANDROID,
	}).Init(g, nil)
	assert.NoError(t, err)
	return rv
}

func androidCfg() *AndroidRepoManagerConfig {
	return &AndroidRepoManagerConfig{
		CommonRepoManagerConfig{
			ChildBranch:  "master",
			ChildPath:    childPath,
			ParentBranch: "master",
		},
	}
}

func setupAndroid(t *testing.T) (context.Context, string, func()) {
	wd, err := ioutil.TempDir("", "")
	assert.NoError(t, err)
	mockRun := exec.CommandCollector{}
	mockRun.SetDelegateRun(func(cmd *exec.Command) error {
		if strings.Contains(cmd.Name, "repo") {
			return nil
		}
		if cmd.Name == "git" {
			var output string
			if cmd.Args[0] == "log" {
				if cmd.Args[1] == "--format=format:%H%x20%ci" {
					output = fmt.Sprintf("%s 2017-03-29 18:29:22 +0000\n%s 2017-03-29 18:29:22 +0000", childCommits[0], childCommits[1])
				}
				if cmd.Args[1] == "-n" && cmd.Args[2] == "1" {
					commit := ""
					for _, c := range childCommits {
						if cmd.Args[len(cmd.Args)-1] == c {
							commit = c
							break
						}
					}
					assert.NotEqual(t, "", commit)
					output = fmt.Sprintf("%s\nparent\nMe (me@google.com)\nsome commit\n1558543876\n", commit)
				}
			} else if cmd.Args[0] == "ls-remote" {
				output = childCommits[0]
			} else if cmd.Args[0] == "merge-base" {
				output = childCommits[1]
			} else if cmd.Args[0] == "rev-list" {
				split := strings.Split(cmd.Args[len(cmd.Args)-1], "..")
				assert.Equal(t, 2, len(split))
				startCommit := split[0]
				endCommit := split[1]
				start, end := -1, -1
				for i := 0; i < len(childCommits); i++ {
					if childCommits[i] == startCommit {
						start = i
					}
					if childCommits[i] == endCommit {
						end = i
					}
				}
				assert.NotEqual(t, -1, start)
				assert.NotEqual(t, -1, end)
				output = strings.Join(childCommits[end:start], "\n")
			}
			n, err := cmd.CombinedOutput.Write([]byte(output))
			assert.NoError(t, err)
			assert.Equal(t, len(output), n)
		}
		return nil
	})
	ctx := exec.NewContext(context.Background(), mockRun.Run)
	cleanup := func() {
		testutils.RemoveAll(t, wd)
	}
	return ctx, wd, cleanup
}

// TestAndroidRepoManager tests all aspects of the RepoManager except for CreateNewRoll.
func TestAndroidRepoManager(t *testing.T) {
	unittest.LargeTest(t)
	ctx, wd, cleanup := setupAndroid(t)
	defer cleanup()
	g := &gerrit_mocks.SimpleGerritInterface{IssueID: androidIssueNum}
	rm, err := NewAndroidRepoManager(ctx, androidCfg(), wd, g, "fake.server.com", "fake-service-account", nil, androidGerrit(t, g), false)
	assert.NoError(t, err)
	assert.NoError(t, SetStrategy(ctx, rm, strategy.ROLL_STRATEGY_BATCH))
	assert.NoError(t, rm.Update(ctx))

	assert.Equal(t, fmt.Sprintf("%s/android_repo/%s", wd, childPath), rm.(*androidRepoManager).childDir)
	assert.Equal(t, childCommits[len(childCommits)-1], rm.LastRollRev())
	assert.Equal(t, childCommits[0], rm.NextRollRev())
}

// TestCreateNewAndroidRoll tests creating a new roll.
func TestCreateNewAndroidRoll(t *testing.T) {
	unittest.LargeTest(t)
	ctx, wd, cleanup := setupAndroid(t)
	defer cleanup()

	g := &gerrit_mocks.SimpleGerritInterface{IssueID: androidIssueNum}
	rm, err := NewAndroidRepoManager(ctx, androidCfg(), wd, g, "fake.server.com", "fake-service-account", nil, androidGerrit(t, g), false)
	assert.NoError(t, err)
	assert.NoError(t, SetStrategy(ctx, rm, strategy.ROLL_STRATEGY_BATCH))
	assert.NoError(t, rm.Update(ctx))

	issue, err := rm.CreateNewRoll(ctx, rm.LastRollRev(), rm.NextRollRev(), androidEmails, "", false)
	assert.NoError(t, err)
	assert.Equal(t, issueNum, issue)
}

func TestExtractBugNumbers(t *testing.T) {
	unittest.SmallTest(t)

	bodyWithTwoBugs := `testing
Test: tested
BUG=skia:123
Bug: skia:456
BUG=b/123
Bug: b/234`
	bugNumbers := ExtractBugNumbers(bodyWithTwoBugs)
	assert.Equal(t, 2, len(bugNumbers))
	assert.True(t, bugNumbers["123"])
	assert.True(t, bugNumbers["234"])

	bodyWithNoBugs := `testing
Test: tested
BUG=skia:123
Bug: skia:456
BUG=ba/123
Bug: bb/234`
	bugNumbers = ExtractBugNumbers(bodyWithNoBugs)
	assert.Equal(t, 0, len(bugNumbers))
}

func TestExtractTestLines(t *testing.T) {
	unittest.SmallTest(t)

	bodyWithThreeTestLines := `testing
Test: tested with 0
testing
BUG=skia:123
Bug: skia:456
Test: tested with 1
BUG=b/123
Bug: b/234

Test: tested with 2
`
	testLines := ExtractTestLines(bodyWithThreeTestLines)
	assert.Equal(t, []string{"Test: tested with 0", "Test: tested with 1", "Test: tested with 2"}, testLines)

	bodyWithNoTestLines := `testing
no test
lines
included
here
`
	testLines = ExtractTestLines(bodyWithNoTestLines)
	assert.Equal(t, 0, len(testLines))
}

// Verify that we ran the PreUploadSteps.
func TestRanPreUploadStepsAndroid(t *testing.T) {
	unittest.LargeTest(t)
	ctx, wd, cleanup := setupAndroid(t)
	defer cleanup()

	g := &gerrit_mocks.SimpleGerritInterface{IssueID: androidIssueNum}
	rm, err := NewAndroidRepoManager(ctx, androidCfg(), wd, g, "fake.server.com", "fake-service-account", nil, androidGerrit(t, g), false)
	assert.NoError(t, err)
	assert.NoError(t, SetStrategy(ctx, rm, strategy.ROLL_STRATEGY_BATCH))
	assert.NoError(t, rm.Update(ctx))

	ran := false
	rm.(*androidRepoManager).preUploadSteps = []PreUploadStep{
		func(context.Context, []string, *http.Client, string) error {
			ran = true
			return nil
		},
	}

	// Create a roll, assert that we ran the PreUploadSteps.
	_, err = rm.CreateNewRoll(ctx, rm.LastRollRev(), rm.NextRollRev(), androidEmails, "", false)
	assert.NoError(t, err)
	assert.True(t, ran)
}

func TestAndroidConfigValidation(t *testing.T) {
	unittest.SmallTest(t)

	cfg := androidCfg()
	assert.NoError(t, cfg.Validate())

	// The only fields come from the nested Configs, so exclude them and
	// verify that we fail validation.
	cfg = &AndroidRepoManagerConfig{}
	assert.Error(t, cfg.Validate())
}
