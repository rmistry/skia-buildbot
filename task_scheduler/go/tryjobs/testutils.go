package tryjobs

import (
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"math/rand"
	"os"
	"path"
	"strconv"
	"time"

	assert "github.com/stretchr/testify/require"
	buildbucket_api "go.chromium.org/luci/common/api/buildbucket/buildbucket/v1"
	"go.skia.org/infra/go/buildbucket"
	depot_tools_testutils "go.skia.org/infra/go/depot_tools/testutils"
	"go.skia.org/infra/go/gerrit"
	"go.skia.org/infra/go/git/repograph"
	git_testutils "go.skia.org/infra/go/git/testutils"
	"go.skia.org/infra/go/isolate"
	"go.skia.org/infra/go/mockhttpclient"
	"go.skia.org/infra/go/sklog"
	"go.skia.org/infra/go/sktest"
	"go.skia.org/infra/go/testutils"
	"go.skia.org/infra/go/testutils/unittest"
	"go.skia.org/infra/task_scheduler/go/cacher"
	"go.skia.org/infra/task_scheduler/go/db/cache"
	"go.skia.org/infra/task_scheduler/go/db/memory"
	"go.skia.org/infra/task_scheduler/go/isolate_cache"
	"go.skia.org/infra/task_scheduler/go/syncer"
	"go.skia.org/infra/task_scheduler/go/task_cfg_cache"
	tcc_testutils "go.skia.org/infra/task_scheduler/go/task_cfg_cache/testutils"
	"go.skia.org/infra/task_scheduler/go/types"
	"go.skia.org/infra/task_scheduler/go/window"
)

const (
	repoBaseName = "skia.git"
	testTasksCfg = `{
  "tasks": {
    "fake-task1": {
      "dependencies": [],
      "dimensions": ["pool:Skia", "os:Ubuntu", "cpu:x86-64-avx2", "gpu:none"],
      "extra_args": [],
      "isolate": "fake1.isolate",
      "priority": 0.8
    },
    "fake-task2": {
      "dependencies": ["fake-task1"],
      "dimensions": ["pool:Skia", "os:Ubuntu", "cpu:x86-64-avx2", "gpu:none"],
      "extra_args": [],
      "isolate": "fake2.isolate",
      "priority": 0.8
    }
  },
  "jobs": {
    "fake-job": {
      "priority": 0.8,
      "tasks": ["fake-task2"]
    }
  }
}`
	gerritUrl      = "https://skia-review.googlesource.com/"
	gerritIssue    = 2112
	gerritPatchset = "3"
	patchProject   = "skia"
	parentProject  = "parent-project"

	fakeGerritUrl = "https://fake-skia-review.googlesource.com"
)

var (
	gerritPatch = types.Patch{
		Server:   gerritUrl,
		Issue:    fmt.Sprintf("%d", gerritIssue),
		Patchset: gerritPatchset,
	}
)

// setup prepares the tests to run. Returns the created temporary dir,
// TryJobIntegrator instance, and URLMock instance.
func setup(t sktest.TestingT) (context.Context, *TryJobIntegrator, *git_testutils.GitBuilder, *mockhttpclient.URLMock, func()) {
	unittest.LargeTest(t)

	ctx := context.Background()

	// Set up the test Git repo.
	gb := git_testutils.GitInit(t, ctx)
	assert.NoError(t, os.MkdirAll(path.Join(gb.Dir(), "infra", "bots"), os.ModePerm))
	tasksJson := path.Join("infra", "bots", "tasks.json")
	gb.Add(ctx, tasksJson, testTasksCfg)
	gb.Add(ctx, path.Join("infra", "bots", "fake1.isolate"), "{}")
	gb.Add(ctx, path.Join("infra", "bots", "fake2.isolate"), "{}")
	gb.Commit(ctx)

	rs := types.RepoState{
		Patch:    gerritPatch,
		Repo:     gb.RepoUrl(),
		Revision: "master",
	}

	// Create a ref for a fake patch.
	gb.CreateFakeGerritCLGen(ctx, rs.Issue, rs.Patchset)

	// Create a second repo, for cross-repo tryjob testing.
	gb2 := git_testutils.GitInit(t, ctx)
	gb2.CommitGen(ctx, "somefile")

	// Create repo map.
	tmpDir, err := ioutil.TempDir("", "")
	assert.NoError(t, err)

	rm, err := repograph.NewLocalMap(ctx, []string{gb.RepoUrl(), gb2.RepoUrl()}, tmpDir)
	assert.NoError(t, err)
	assert.NoError(t, rm.Update(ctx))

	// Set up other TryJobIntegrator inputs.
	window, err := window.New(time.Hour, 100, rm)
	assert.NoError(t, err)
	btProject, btInstance, btCleanup := tcc_testutils.SetupBigTable(t)
	taskCfgCache, err := task_cfg_cache.NewTaskCfgCache(ctx, rm, btProject, btInstance, nil)
	assert.NoError(t, err)
	d := memory.NewInMemoryDB(nil)
	mock := mockhttpclient.NewURLMock()
	projectRepoMapping := map[string]string{
		patchProject:  gb.RepoUrl(),
		parentProject: gb2.RepoUrl(),
	}

	gitcookies := path.Join(tmpDir, "gitcookies_fake")
	assert.NoError(t, ioutil.WriteFile(gitcookies, []byte(".googlesource.com\tTRUE\t/\tTRUE\t123\to\tgit-user.google.com=abc123"), os.ModePerm))
	g, err := gerrit.NewGerrit(fakeGerritUrl, gitcookies, mock.Client())
	assert.NoError(t, err)

	depotTools := depot_tools_testutils.GetDepotTools(t, ctx)
	s := syncer.New(ctx, rm, depotTools, tmpDir, syncer.DEFAULT_NUM_WORKERS)
	isolateClient, err := isolate.NewClient(tmpDir, isolate.ISOLATE_SERVER_URL_FAKE)
	assert.NoError(t, err)
	btCleanupIsolate := isolate_cache.SetupSharedBigTable(t, btProject, btInstance)
	isolateCache, err := isolate_cache.New(ctx, btProject, btInstance, nil)
	assert.NoError(t, err)
	chr := cacher.New(s, taskCfgCache, isolateClient, isolateCache)
	jCache, err := cache.NewJobCache(d, window, cache.GitRepoGetRevisionTimestamp(rm))
	assert.NoError(t, err)
	integrator, err := NewTryJobIntegrator(API_URL_TESTING, BUCKET_TESTING, "fake-server", mock.Client(), d, jCache, projectRepoMapping, rm, taskCfgCache, chr, g)
	assert.NoError(t, err)

	return ctx, integrator, gb, mock, func() {
		testutils.AssertCloses(t, isolateClient)
		testutils.AssertCloses(t, taskCfgCache)
		testutils.RemoveAll(t, tmpDir)
		gb.Cleanup()
		btCleanupIsolate()
		btCleanup()
		assert.NoError(t, s.Close())
	}
}

func Params(t sktest.TestingT, builder, project, revision, server, issue, patchset string) buildbucket.Parameters {
	p := buildbucket.Parameters{
		BuilderName: builder,
		Properties: buildbucket.Properties{
			PatchProject: project,
			Revision:     revision,
		},
	}
	issueInt, err := strconv.Atoi(issue)
	assert.NoError(t, err)
	p.Properties.PatchStorage = "gerrit"
	p.Properties.Gerrit = server
	p.Properties.GerritIssue = int64(issueInt)
	p.Properties.GerritPatchset = patchset
	return p
}

func Build(t sktest.TestingT, now time.Time) *buildbucket_api.LegacyApiCommonBuildMessage {
	return &buildbucket_api.LegacyApiCommonBuildMessage{
		Bucket:            BUCKET_TESTING,
		CreatedBy:         "tests",
		CreatedTs:         now.Unix() * 1000000,
		Id:                rand.Int63(),
		LeaseExpirationTs: now.Add(LEASE_DURATION_INITIAL).Unix() * 1000000,
		LeaseKey:          987654321,
		ParametersJson:    testutils.MarshalJSON(t, Params(t, "fake-job", patchProject, "master", gerritPatch.Server, gerritPatch.Issue, gerritPatch.Patchset)),
		Status:            "SCHEDULED",
	}
}

func tryjob(repoName string) *types.Job {
	return &types.Job{
		BuildbucketBuildId:  rand.Int63(),
		BuildbucketLeaseKey: rand.Int63(),
		Created:             time.Now(),
		Name:                "fake-name",
		RepoState: types.RepoState{
			Patch: types.Patch{
				Server:   "fake-server",
				Issue:    "fake-issue",
				Patchset: "fake-patchset",
			},
			Repo:     repoName,
			Revision: "fake-revision",
		},
	}
}

type errMsg struct {
	Message string `json:"message"`
}

type heartbeat struct {
	BuildId           string `json:"build_id"`
	LeaseExpirationTs string `json:"lease_expiration_ts"`
	LeaseKey          string `json:"lease_key"`
}

type heartbeatResp struct {
	BuildId string  `json:"build_id,omitempty"`
	Error   *errMsg `json:"error,omitempty"`
}

func MockHeartbeats(t sktest.TestingT, mock *mockhttpclient.URLMock, now time.Time, jobs []*types.Job, resps map[string]*heartbeatResp) {
	// Create the request data.
	expiry := fmt.Sprintf("%d", now.Add(LEASE_DURATION).Unix()*1000000)
	heartbeats := make([]*heartbeat, 0, len(jobs))
	for _, j := range jobs {
		heartbeats = append(heartbeats, &heartbeat{
			BuildId:           fmt.Sprintf("%d", j.BuildbucketBuildId),
			LeaseExpirationTs: expiry,
			LeaseKey:          fmt.Sprintf("%d", j.BuildbucketLeaseKey),
		})
	}
	req, err := json.Marshal(&struct {
		Heartbeats []*heartbeat `json:"heartbeats"`
	}{
		Heartbeats: heartbeats,
	})
	assert.NoError(t, err)
	req = append(req, []byte("\n")...)

	// Create the response data.
	if resps == nil {
		resps = map[string]*heartbeatResp{}
	}
	results := make([]*heartbeatResp, 0, len(jobs))
	for _, j := range jobs {
		r, ok := resps[j.Id]
		if !ok {
			r = &heartbeatResp{
				BuildId: fmt.Sprintf("%d", j.BuildbucketBuildId),
			}
		}
		results = append(results, r)
	}
	resp, err := json.Marshal(&struct {
		Results []*heartbeatResp `json:"results"`
	}{
		Results: results,
	})
	assert.NoError(t, err)
	resp = append(resp, []byte("\n")...)

	mock.MockOnce(fmt.Sprintf("%sheartbeat?alt=json&prettyPrint=false", API_URL_TESTING), mockhttpclient.MockPostDialogue("application/json", req, resp))
}

func MockCancelBuild(mock *mockhttpclient.URLMock, id int64, msg string, err error) {
	req := []byte(fmt.Sprintf("{\"result_details_json\":\"{\\\"message\\\":\\\"%s\\\"}\"}\n", msg))
	respStr := "{}"
	if err != nil {
		respStr = fmt.Sprintf("{\"error\": {\"message\": \"%s\"}}", err)
	}
	resp := []byte(respStr)
	mock.MockOnce(fmt.Sprintf("%sbuilds/%d/cancel?alt=json&prettyPrint=false", API_URL_TESTING, id), mockhttpclient.MockPostDialogue("application/json", req, resp))
}

func MockTryLeaseBuild(mock *mockhttpclient.URLMock, id int64, err error) {
	req := mockhttpclient.DONT_CARE_REQUEST
	respStr := fmt.Sprintf("{\"build\": {\"lease_key\": \"%d\"}}", 987654321)
	if err != nil {
		respStr = fmt.Sprintf("{\"error\": {\"message\": \"%s\"}}", err)
	}
	resp := []byte(respStr)
	mock.MockOnce(fmt.Sprintf("%sbuilds/%d/lease?alt=json&prettyPrint=false", API_URL_TESTING, id), mockhttpclient.MockPostDialogue("application/json", req, resp))
}

func MockJobStarted(mock *mockhttpclient.URLMock, id int64, err error) {
	// We have to use this because we don't know what the Job ID is going to
	// be until after it's inserted into the DB.
	req := mockhttpclient.DONT_CARE_REQUEST
	resp := []byte("{}")
	if err != nil {
		resp = []byte(fmt.Sprintf("{\"error\":{\"message\":\"%s\"}}", err.Error()))
	}
	mock.MockOnce(fmt.Sprintf("%sbuilds/%d/start?alt=json&prettyPrint=false", API_URL_TESTING, id), mockhttpclient.MockPostDialogue("application/json", req, resp))
}

func serializeJob(j *types.Job) string {
	jobBytes, err := json.Marshal(j)
	if err != nil {
		sklog.Fatal(err)
	}
	escape, err := json.Marshal(string(jobBytes))
	if err != nil {
		sklog.Fatal(err)
	}
	return string(escape[1 : len(escape)-1])
}

func MockJobSuccess(mock *mockhttpclient.URLMock, j *types.Job, now time.Time, expectErr error, dontCareRequest bool) {
	req := mockhttpclient.DONT_CARE_REQUEST
	if !dontCareRequest {
		req = []byte(fmt.Sprintf("{\"lease_key\":\"%d\",\"result_details_json\":\"{\\\"job\\\":%s}\",\"url\":\"fake-server/job/%s\"}\n", j.BuildbucketLeaseKey, serializeJob(j), j.Id))
	}
	resp := []byte("{}")
	if expectErr != nil {
		resp = []byte(fmt.Sprintf("{\"error\":{\"message\":\"%s\"}}", expectErr.Error()))
	}
	mock.MockOnce(fmt.Sprintf("%sbuilds/%d/succeed?alt=json&prettyPrint=false", API_URL_TESTING, j.BuildbucketBuildId), mockhttpclient.MockPostDialogue("application/json", req, resp))
}

func MockJobFailure(mock *mockhttpclient.URLMock, j *types.Job, now time.Time, expectErr error) {
	req := []byte(fmt.Sprintf("{\"failure_reason\":\"BUILD_FAILURE\",\"lease_key\":\"%d\",\"result_details_json\":\"{\\\"job\\\":%s}\",\"url\":\"fake-server/job/%s\"}\n", j.BuildbucketLeaseKey, serializeJob(j), j.Id))
	resp := []byte("{}")
	if expectErr != nil {
		resp = []byte(fmt.Sprintf("{\"error\":{\"message\":\"%s\"}}", expectErr.Error()))
	}
	mock.MockOnce(fmt.Sprintf("%sbuilds/%d/fail?alt=json&prettyPrint=false", API_URL_TESTING, j.BuildbucketBuildId), mockhttpclient.MockPostDialogue("application/json", req, resp))
}

func MockJobMishap(mock *mockhttpclient.URLMock, j *types.Job, now time.Time, expectErr error) {
	req := []byte(fmt.Sprintf("{\"failure_reason\":\"INFRA_FAILURE\",\"lease_key\":\"%d\",\"result_details_json\":\"{\\\"job\\\":%s}\",\"url\":\"fake-server/job/%s\"}\n", j.BuildbucketLeaseKey, serializeJob(j), j.Id))
	resp := []byte("{}")
	if expectErr != nil {
		resp = []byte(fmt.Sprintf("{\"error\":{\"message\":\"%s\"}}", expectErr.Error()))
	}
	mock.MockOnce(fmt.Sprintf("%sbuilds/%d/fail?alt=json&prettyPrint=false", API_URL_TESTING, j.BuildbucketBuildId), mockhttpclient.MockPostDialogue("application/json", req, resp))
}

func MockPeek(mock *mockhttpclient.URLMock, builds []*buildbucket_api.LegacyApiCommonBuildMessage, now time.Time, cursor, nextcursor string, err error) {
	resp := buildbucket_api.LegacyApiSearchResponseMessage{
		Builds:     builds,
		NextCursor: nextcursor,
	}
	if err != nil {
		resp.Error = &buildbucket_api.LegacyApiErrorMessage{
			Message: err.Error(),
		}
	}
	respBytes, err := json.Marshal(&resp)
	if err != nil {
		panic(err)
	}
	mock.MockOnce(fmt.Sprintf("%speek?alt=json&bucket=%s&max_builds=50&prettyPrint=false&start_cursor=%s", API_URL_TESTING, BUCKET_TESTING, cursor), mockhttpclient.MockGetDialogue(respBytes))
}
