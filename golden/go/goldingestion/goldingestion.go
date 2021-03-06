package goldingestion

import (
	"context"
	"fmt"
	"net/http"

	"go.skia.org/infra/go/eventbus"
	"go.skia.org/infra/go/ingestion"
	"go.skia.org/infra/go/sharedconfig"
	"go.skia.org/infra/go/skerr"
	"go.skia.org/infra/go/sklog"
	tracedb "go.skia.org/infra/go/trace/db"
	"go.skia.org/infra/go/vcsinfo"
	"go.skia.org/infra/golden/go/config"
	"go.skia.org/infra/golden/go/types"
)

const (
	// Configuration option that identifies the address of the traceDB service.
	CONFIG_TRACESERVICE = "TraceService"

	// Configuration option for the secondary repository.
	CONFIG_SECONDARY_REPO = "SecondaryRepoURL"

	// Configuration option to define the regular expression to extract the
	// commit from the secondary repo. The provided regular expression must
	// contain exactly one group which maps to the commit in the DEPS file.
	CONFIG_SECONDARY_REG_EX = "SecondaryRegEx"
)

// Register the processor with the ingestion framework.
func init() {
	ingestion.Register(config.CONSTRUCTOR_GOLD, newGoldProcessor)
}

// goldProcessor implements the ingestion.Processor interface for gold.
type goldProcessor struct {
	traceDB tracedb.DB
	vcs     vcsinfo.VCS
}

// implements the ingestion.Constructor signature.
func newGoldProcessor(vcs vcsinfo.VCS, config *sharedconfig.IngesterConfig, client *http.Client, eventBus eventbus.EventBus) (ingestion.Processor, error) {
	traceDB, err := tracedb.NewTraceServiceDBFromAddress(config.ExtraParams[CONFIG_TRACESERVICE], types.GoldenTraceBuilder)
	if err != nil {
		return nil, err
	}

	ret := &goldProcessor{
		traceDB: traceDB,
		vcs:     vcs,
	}
	return ret, nil
}

// See ingestion.Processor interface.
func (g *goldProcessor) Process(ctx context.Context, resultsFile ingestion.ResultFileLocation) error {
	dmResults, err := processDMResults(resultsFile)
	if err != nil {
		return skerr.Fmt("could not process results file: %s", err)
	}

	if len(dmResults.Results) == 0 {
		sklog.Infof("ignoring file %s because it has no results", resultsFile.Name())
		return ingestion.IgnoreResultsFileErr
	}

	var commit *vcsinfo.LongCommit = nil
	// If the target commit is not in the primary repository we look it up
	// in the secondary that has the primary as a dependency.
	targetHash, err := g.getCanonicalCommitHash(ctx, dmResults.GitHash)
	if err != nil {
		if err == ingestion.IgnoreResultsFileErr {
			return ingestion.IgnoreResultsFileErr
		}
		return skerr.Fmt("could not identify canonical commit from %q: %s", dmResults.GitHash, err)
	}

	commit, err = g.vcs.Details(ctx, targetHash, true)
	if err != nil {
		return skerr.Fmt("could not get details for git commit %q: %s", targetHash, err)
	}

	if !commit.Branches["master"] {
		sklog.Warningf("Commit %s is not in master branch. Got branches: %v", commit.Hash, commit.Branches)
		return ingestion.IgnoreResultsFileErr
	}

	// Add the column to the trace db.
	cid, err := g.getCommitID(commit)
	if err != nil {
		return skerr.Fmt("could not get trace db id: %s", err)
	}

	// Get the entries that should be added to the tracedb.
	entries, err := extractTraceDBEntries(dmResults)
	if err != nil {
		return skerr.Fmt("could not create entries for results: %s", err)
	}

	// Write the result to the tracedb.
	err = g.traceDB.Add(cid, entries)
	if err != nil {
		return skerr.Fmt("could not add to tracedb: %s", err)
	}
	return nil
}

// See ingestion.Processor interface.
func (g *goldProcessor) BatchFinished() error { return nil }

// getCommitID extracts the commitID from the given commit.
func (g *goldProcessor) getCommitID(commit *vcsinfo.LongCommit) (*tracedb.CommitID, error) {
	return &tracedb.CommitID{
		Timestamp: commit.Timestamp.Unix(),
		ID:        commit.Hash,
		Source:    "master",
	}, nil
}

// getCanonicalCommitHash returns the commit hash in the primary repository. If the given
// target hash is not in the primary repository it will try and find it in the secondary
// repository which has the primary as a dependency.
func (g *goldProcessor) getCanonicalCommitHash(ctx context.Context, targetHash string) (string, error) {
	// If it is not in the primary repo.
	if !isCommit(ctx, g.vcs, targetHash) {
		// Extract the commit.
		foundCommit, err := g.vcs.ResolveCommit(ctx, targetHash)
		if err != nil && err != vcsinfo.NoSecondaryRepo {
			return "", fmt.Errorf("Unable to resolve commit %s in primary or secondary repo. Got err: %s", targetHash, err)
		}

		if foundCommit == "" {
			if err == vcsinfo.NoSecondaryRepo {
				sklog.Warningf("Unable to find commit %s in primary or secondary repo.", targetHash)
			} else {
				sklog.Warningf("Unable to find commit %s in primary repo and no secondary configured.", targetHash)
			}
			return "", ingestion.IgnoreResultsFileErr
		}

		// Check if the found commit is actually in the primary repository. This could indicate misconfiguration
		// of the secondary repo.
		if !isCommit(ctx, g.vcs, foundCommit) {
			return "", fmt.Errorf("Found invalid commit %s in secondary repo at commit %s. Not contained in primary repo.", foundCommit, targetHash)
		}
		sklog.Infof("Commit translation: %s -> %s", targetHash, foundCommit)
		targetHash = foundCommit
	}
	return targetHash, nil
}

// isCommit returns true if the given commit is in vcs.
func isCommit(ctx context.Context, vcs vcsinfo.VCS, commitHash string) bool {
	ret, err := vcs.Details(ctx, commitHash, false)
	return (err == nil) && (ret != nil)
}
