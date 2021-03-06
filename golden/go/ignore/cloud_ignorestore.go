package ignore

import (
	"context"
	"fmt"
	"sort"
	"sync/atomic"

	"cloud.google.com/go/datastore"
	"go.skia.org/infra/go/ds"
	"go.skia.org/infra/go/skerr"
	"go.skia.org/infra/go/sklog"
	"go.skia.org/infra/golden/go/dsutil"
	"go.skia.org/infra/golden/go/expstorage"
	"go.skia.org/infra/golden/go/types"
	"golang.org/x/sync/errgroup"
)

// cloudIgnoreStore implements the IgnoreStore interface
type cloudIgnoreStore struct {
	client         *datastore.Client
	recentKeysList *dsutil.RecentKeysList
	revision       int64
	lastCpxTile    types.ComplexTile
	expStore       expstorage.ExpectationsStore
	cpxTileStream  <-chan types.ComplexTile
}

// NewCloudIgnoreStore returns an IgnoreStore instance that is backed by Cloud Datastore.
func NewCloudIgnoreStore(client *datastore.Client, expStore expstorage.ExpectationsStore, tileStream <-chan types.ComplexTile) (IgnoreStore, error) {
	if client == nil {
		return nil, skerr.Fmt("Received nil for datastore client.")
	}

	containerKey := ds.NewKey(ds.HELPER_RECENT_KEYS)
	containerKey.Name = "ignore:recent-keys"

	store := &cloudIgnoreStore{
		client:         client,
		recentKeysList: dsutil.NewRecentKeysList(client, containerKey, dsutil.DefaultConsistencyDelta),
		expStore:       expStore,
		cpxTileStream:  tileStream,
	}
	return store, nil
}

// Create implements the IgnoreStore interface.
func (c *cloudIgnoreStore) Create(ignoreRule *IgnoreRule) error {
	createFn := func(tx *datastore.Transaction) error {
		key := dsutil.TimeSortableKey(ds.IGNORE_RULE, 0)
		ignoreRule.ID = key.ID

		// Add the new rule and put its key with the recently added keys.
		if _, err := tx.Put(key, ignoreRule); err != nil {
			return err
		}

		return c.recentKeysList.Add(tx, key)
	}

	// Run the relevant updates in a transaction.
	_, err := c.client.RunInTransaction(context.TODO(), createFn)

	// TODO(stephana): Look into removing the revision feature. I don't think
	// this is really necessary going forward.

	if err == nil {
		atomic.AddInt64(&c.revision, 1)
	}
	return err
}

// List implements the IgnoreStore interface.
func (c *cloudIgnoreStore) List(addCounts bool) ([]*IgnoreRule, error) {
	ctx := context.TODO()
	var egroup errgroup.Group
	var queriedKeys []*datastore.Key
	egroup.Go(func() error {
		// Query all entities.
		query := ds.NewQuery(ds.IGNORE_RULE).KeysOnly()
		var err error
		queriedKeys, err = c.client.GetAll(ctx, query, nil)
		return err
	})

	var recently *dsutil.Recently
	egroup.Go(func() error {
		var err error
		recently, err = c.recentKeysList.GetRecent()
		return err
	})

	if err := egroup.Wait(); err != nil {
		return nil, skerr.Fmt("Error getting keys of ignore rules: %s", err)
	}

	// Merge the keys to get all of the current keys.
	allKeys := recently.Combine(queriedKeys)
	if len(allKeys) == 0 {
		return []*IgnoreRule{}, nil
	}

	ret := make([]*IgnoreRule, len(allKeys))
	if err := c.client.GetMulti(ctx, allKeys, ret); err != nil {
		return nil, err
	}
	sort.Slice(ret, func(i, j int) bool { return ret[i].Expires.Before(ret[j].Expires) })

	if addCounts {
		var err error
		c.lastCpxTile, err = addIgnoreCounts(ret, c, c.lastCpxTile, c.expStore, c.cpxTileStream)
		if err != nil {
			sklog.Errorf("Unable to add counts to ignore list result: %s", err)
		}
	}

	return ret, nil
}

// Update implements the IgnoreStore interface.
func (c *cloudIgnoreStore) Update(id int64, rule *IgnoreRule) error {
	ctx := context.TODO()
	key := ds.NewKey(ds.IGNORE_RULE)
	key.ID = id
	_, err := c.client.Mutate(ctx, datastore.NewUpdate(key, rule))
	if err == nil {
		atomic.AddInt64(&c.revision, 1)
	}
	return err
}

// Delete implements the IgnoreStore interface.
func (c *cloudIgnoreStore) Delete(id int64) (int, error) {
	if id <= 0 {
		return 0, skerr.Fmt("Given id does not exist: %d", id)
	}

	deleteFn := func(tx *datastore.Transaction) error {
		key := ds.NewKey(ds.IGNORE_RULE)
		key.ID = id

		ignoreRule := &IgnoreRule{}
		if err := tx.Get(key, ignoreRule); err != nil {
			return err
		}

		if err := tx.Delete(key); err != nil {
			return err
		}

		return c.recentKeysList.Delete(tx, key)
	}

	// Run the relevant updates in a transaction.
	_, err := c.client.RunInTransaction(context.TODO(), deleteFn)
	if err != nil {
		// Don't report an error if the item did not exist.
		if err == datastore.ErrNoSuchEntity {
			sklog.Warningf("Could not delete ignore with id %d because it did not exist", id)
			return 0, nil
		}
		return 0, err
	}

	atomic.AddInt64(&c.revision, 1)
	return 1, nil
}

// Revision implements the IgnoreStore interface.
func (c *cloudIgnoreStore) Revision() int64 {
	return atomic.LoadInt64(&c.revision)
}

// BuildRuleMatcher implements the IgnoreStore interface.
func (c *cloudIgnoreStore) BuildRuleMatcher() (RuleMatcher, error) {
	return buildRuleMatcher(c)
}

// TODO(kjlubick): Add unit tests to addIgnoreCounts using mocks

// addIgnoreCounts adds counts for the current tile to the given list of rules.
func addIgnoreCounts(rules []*IgnoreRule, ignoreStore IgnoreStore, lastCpxTile types.ComplexTile, expStore expstorage.ExpectationsStore, tileStream <-chan types.ComplexTile) (types.ComplexTile, error) {
	if (expStore == nil) || (tileStream == nil) {
		return nil, fmt.Errorf("Either expStore or tileStream is nil. Cannot count ignores.")
	}

	exp, err := expStore.Get()
	if err != nil {
		return nil, err
	}

	ignoreMatcher, err := ignoreStore.BuildRuleMatcher()
	if err != nil {
		return nil, err
	}

	// Get the next tile.
	var cpxTile types.ComplexTile = nil
	select {
	case cpxTile = <-tileStream:
	default:
		cpxTile = lastCpxTile
	}
	if cpxTile == nil {
		return nil, fmt.Errorf("No tile available to count ignores")
	}

	// Count the untriaged digests in HEAD.
	// matchingDigests[rule.ID]map["testname:digest"]bool
	matchingDigests := make(map[int64]map[string]bool, len(rules))
	rulesByDigest := map[string]map[int64]bool{}
	tileWithIgnores := cpxTile.GetTile(types.IncludeIgnoredTraces)
	for _, trace := range tileWithIgnores.Traces {
		gTrace := trace.(*types.GoldenTrace)
		if matchRules, ok := ignoreMatcher(gTrace.Keys); ok {
			testName := gTrace.TestName()
			if digest := gTrace.LastDigest(); digest != types.MISSING_DIGEST && (exp.Classification(testName, digest) == types.UNTRIAGED) {
				k := string(testName) + ":" + string(digest)
				for _, r := range matchRules {
					// Add the digest to all matching rules.
					if t, ok := matchingDigests[r.ID]; ok {
						t[k] = true
					} else {
						matchingDigests[r.ID] = map[string]bool{k: true}
					}

					// Add the rule to the test-digest.
					if t, ok := rulesByDigest[k]; ok {
						t[r.ID] = true
					} else {
						rulesByDigest[k] = map[int64]bool{r.ID: true}
					}
				}
			}
		}
	}

	for _, r := range rules {
		r.Count = len(matchingDigests[r.ID])
		r.ExclusiveCount = 0
		for testDigestKey := range matchingDigests[r.ID] {
			// If exactly this one rule matches then account for it.
			if len(rulesByDigest[testDigestKey]) == 1 {
				r.ExclusiveCount++
			}
		}
	}
	return cpxTile, nil
}
