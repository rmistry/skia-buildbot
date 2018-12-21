package bt_testutil

import (
	"fmt"

	"github.com/google/uuid"
	assert "github.com/stretchr/testify/require"
	"go.skia.org/infra/go/bt"
	"go.skia.org/infra/go/testutils"
)

func SetupBigTable(t testutils.TestingT, cfgs ...bt.TableConfig) (string, string, func()) {
	project := "test-project"
	instance := fmt.Sprintf("test-instance-%s", uuid.New())
	assert.NoError(t, bt.InitBigtable(project, instance, cfgs...))
	return project, instance, func() {
		assert.NoError(t, bt.DeleteTables(project, instance, cfgs...))
	}
}