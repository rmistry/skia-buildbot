package memory

import (
	"os"
	"testing"

	"go.skia.org/infra/go/deepequal"
	"go.skia.org/infra/go/testutils/unittest"
	"go.skia.org/infra/task_scheduler/go/db"
)

func TestMain(m *testing.M) {
	db.AssertDeepEqual = deepequal.AssertDeepEqual
	os.Exit(m.Run())
}

func TestInMemoryTaskDB(t *testing.T) {
	unittest.SmallTest(t)
	db.TestTaskDB(t, NewInMemoryTaskDB(nil))
}

func TestInMemoryTaskDBConcurrentUpdate(t *testing.T) {
	unittest.SmallTest(t)
	db.TestTaskDBConcurrentUpdate(t, NewInMemoryTaskDB(nil))
}

func TestInMemoryTaskDBUpdateTasksWithRetries(t *testing.T) {
	unittest.SmallTest(t)
	db.TestUpdateTasksWithRetries(t, NewInMemoryTaskDB(nil))
}

func TestInMemoryTaskDBGetTasksFromDateRangeByRepo(t *testing.T) {
	unittest.SmallTest(t)
	db.TestTaskDBGetTasksFromDateRangeByRepo(t, NewInMemoryTaskDB(nil))
}

func TestInMemoryTaskDBGetTasksFromWindow(t *testing.T) {
	unittest.LargeTest(t)
	db.TestTaskDBGetTasksFromWindow(t, NewInMemoryTaskDB(nil))
}

func TestInMemoryUpdateDBFromSwarmingTask(t *testing.T) {
	unittest.SmallTest(t)
	db.TestUpdateDBFromSwarmingTask(t, NewInMemoryTaskDB(nil))
}

func TestInMemoryUpdateDBFromSwarmingTaskTryjob(t *testing.T) {
	unittest.SmallTest(t)
	db.TestUpdateDBFromSwarmingTaskTryJob(t, NewInMemoryTaskDB(nil))
}

func TestInMemoryJobDB(t *testing.T) {
	unittest.SmallTest(t)
	db.TestJobDB(t, NewInMemoryJobDB(nil))
}

func TestInMemoryJobDBConcurrentUpdate(t *testing.T) {
	unittest.SmallTest(t)
	db.TestJobDBConcurrentUpdate(t, NewInMemoryJobDB(nil))
}
