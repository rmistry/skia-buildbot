package types

import (
	"testing"

	assert "github.com/stretchr/testify/require"
	"go.skia.org/infra/go/testutils/unittest"
)

func TestExpString(t *testing.T) {
	unittest.SmallTest(t)

	te := Expectations{
		"beta": map[Digest]Label{
			"hash1": POSITIVE,
			"hash3": NEGATIVE,
			"hash2": UNTRIAGED,
		},
		"alpha": map[Digest]Label{
			"hashB": UNTRIAGED,
			"hashA": NEGATIVE,
			"hashC": UNTRIAGED,
		},
	}

	assert.Equal(t, `alpha:
	hashA : negative
	hashB : untriaged
	hashC : untriaged
beta:
	hash1 : positive
	hash2 : untriaged
	hash3 : negative
`, te.String())
}
