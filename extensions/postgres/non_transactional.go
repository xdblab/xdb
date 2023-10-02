package postgres

import (
	"context"
	"github.com/xdblab/xdb/extensions"
)

const selectCurrentExecutionQuery = `SELECT
	ce.process_execution_id, e.is_current, e.status, e.start_time, e.timeout_seconds, e.history_event_id_sequence, e.state_id_sequence, e.info
	FROM xdb_sys_current_process_executions ce
	INNER JOIN xdb_sys_process_executions e ON e.process_id = ce.process_id
	WHERE ce.namespace = $1 AND ce.process_id = $2`

func (d dbSession) SelectCurrentProcessExecution(ctx context.Context, namespace, processId string) (*extensions.ProcessExecutionRow, error) {
	var row extensions.ProcessExecutionRow
	err := d.db.GetContext(ctx, &row, selectCurrentExecutionQuery, namespace, processId)
	row.Namespace = namespace
	row.ProcessId = processId
	row.StartTime = FromPostgresDateTime(row.StartTime)
	return &row, err
}

const selectAsyncStateExecutionForUpdateQuery = `SELECT wait_until_status, execute_status, version as previous_version
	FROM xdb_sys_async_state_executions WHERE process_execution_id=$1 AND state_id=$2 AND state_id_sequence=$3`

func (d dbSession) SelectAsyncStateExecutionForUpdate(ctx context.Context, filter extensions.AsyncStateExecutionSelectFilter) (*extensions.AsyncStateExecutionRowForUpdate, error) {
	var row extensions.AsyncStateExecutionRowForUpdate
	err := d.db.GetContext(ctx, &row, selectProcessExecutionForUpdateQuery, filter.ProcessExecutionId, filter.StateId, filter.StateIdSequence)
	row.ProcessExecutionId = filter.ProcessExecutionId
	row.StateId = filter.StateId
	row.StateIdSequence = filter.StateIdSequence
	return &row, err
}

const batchSelectWorkerTasksOfFirstPageQuery = `SELECT shard_id, task_sequence, process_execution_id, state_id, state_id_sequence, task_type
	FROM xdb_sys_worker_tasks WHERE shard_id = $1 ORDER BY task_sequence ASC LIMIT $2`

func (d dbSession) BatchSelectWorkerTasksOfFirstPage(ctx context.Context, shardId int32, pageSize int32) ([]extensions.WorkerTaskRow, error) {
	var rows []extensions.WorkerTaskRow
	err := d.db.SelectContext(ctx, &rows, batchSelectWorkerTasksOfFirstPageQuery, shardId, pageSize)
	return rows, err
}

const batchDeleteWorkerTaskQuery = `DELETE FROM xdb_sys_worker_tasks WHERE shard_id = $1 AND task_sequence <= $2`

func (d dbSession) BatchDeleteWorkerTask(ctx context.Context, filter extensions.WorkerTaskRangeDeleteFilter) error {
	_, err := d.db.ExecContext(ctx, batchSelectWorkerTasksOfFirstPageQuery, filter.ShardId, filter.MaxTaskSequenceInclusive)
	return err
}
