package diffstore

import (
	"fmt"
	"os"
	"path"
	"path/filepath"
	"testing"

	assert "github.com/stretchr/testify/require"
	"go.skia.org/infra/go/fileutil"
	"go.skia.org/infra/go/sktest"
	"go.skia.org/infra/go/testutils"
	"go.skia.org/infra/go/testutils/unittest"
	"go.skia.org/infra/go/tiling"
	"go.skia.org/infra/golden/go/types"
)

const (
	// TEST_IMG_DIGEST needs to be stored in the secondary bucket.
	TEST_IMG_DIGEST = "abc-test-image-digest-xyz"
)

func TestImageLoader(t *testing.T) {
	unittest.LargeTest(t)

	mapper := GoldDiffStoreMapper{}
	workingDir, tile, imageLoader, cleanup := getImageLoaderAndTile(t, mapper)
	defer cleanup()

	// Iterate over the tile and get all the digests
	digestSet := types.DigestSet{}
	for _, trace := range tile.Traces {
		gt := trace.(*types.GoldenTrace)
		for _, val := range gt.Digests {
			if val != types.MISSING_DIGEST {
				digestSet[val] = true
			}
		}
	}

	// Prefetch the images synchronously.
	digests := digestSet.Keys()[:100]
	imageLoader.Warm(1, digests, true)

	// Make sure they are all on disk.
	for _, digest := range digests {
		assert.True(t, fileutil.FileExists(fileutil.TwoLevelRadixPath(workingDir, getDigestImageFileName(digest))))
	}

	// Fetch images from the secondary bucket.
	_, _, err := imageLoader.Get(1, types.DigestSlice{TEST_IMG_DIGEST})
	assert.NoError(t, err)
	_, _, err = imageLoader.Get(1, types.DigestSlice{"some-image-that-does-not-exist-at-all-in-any-bucket"})
	assert.Error(t, err)
}

// Calls TwoLevelRadixPath to create the local image file path.
func DefaultImagePath(baseDir, imageID string) string {
	imagePath := fmt.Sprintf("%s.%s", imageID, IMG_EXTENSION)
	return fileutil.TwoLevelRadixPath(baseDir, imagePath)
}

func getImageLoaderAndTile(t sktest.TestingT, mapper DiffStoreMapper) (string, *tiling.Tile, *ImageLoader, func()) {
	w, cleanup := testutils.TempDir(t)
	baseDir := path.Join(w, TEST_DATA_BASE_DIR+"-imgloader")
	client, tile := getSetupAndTile(t, baseDir)

	workingDir := filepath.Join(baseDir, "images")
	assert.Nil(t, os.Mkdir(workingDir, 0777))

	imgCacheCount, _ := getCacheCounts(10)
	gsBuckets := []string{TEST_GCS_BUCKET_NAME, TEST_GCS_SECONDARY_BUCKET}
	imgLoader, err := NewImgLoader(client, baseDir, workingDir, gsBuckets, TEST_GCS_IMAGE_DIR, imgCacheCount, mapper)
	assert.NoError(t, err)
	return workingDir, tile, imgLoader, cleanup
}
