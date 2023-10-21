// Copyright 2023 XDBLab organization
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package sql

import (
	"context"

	"github.com/xdblab/xdb-apis/goapi/xdbapi"
	"github.com/xdblab/xdb/common/uuid"
	"github.com/xdblab/xdb/extensions"
	"github.com/xdblab/xdb/persistence"
)

func insertAsyncStateExecution(
	ctx context.Context,
	tx extensions.SQLTransaction,
	processExecutionId uuid.UUID,
	stateId string,
	stateIdSeq int,
	stateConfig *xdbapi.AsyncStateConfig,
	stateInput []byte,
	stateInfo []byte,
) error {
	stateRow := extensions.AsyncStateExecutionRow{
		ProcessExecutionId: processExecutionId,
		StateId:            stateId,
		StateIdSequence:    int32(stateIdSeq),
		// the waitUntil/execute status will be set later

		LastFailure:     nil,
		PreviousVersion: 1,
		Input:           stateInput,
		Info:            stateInfo,
	}

	if stateConfig.GetSkipWaitUntil() {
		stateRow.WaitUntilStatus = persistence.StateExecutionStatusSkipped
		stateRow.ExecuteStatus = persistence.StateExecutionStatusRunning
	} else {
		stateRow.WaitUntilStatus = persistence.StateExecutionStatusRunning
		stateRow.ExecuteStatus = persistence.StateExecutionStatusUndefined
	}

	return tx.InsertAsyncStateExecution(ctx, stateRow)
}

func insertImmediateTask(
	ctx context.Context,
	tx extensions.SQLTransaction,
	processExecutionId uuid.UUID,
	stateId string,
	stateIdSeq int,
	stateConfig *xdbapi.AsyncStateConfig,
	shardId int32,
) error {
	immediateTaskRow := extensions.ImmediateTaskRowForInsert{
		ShardId:            shardId,
		ProcessExecutionId: processExecutionId,
		StateId:            stateId,
		StateIdSequence:    int32(stateIdSeq),
	}
	if stateConfig.GetSkipWaitUntil() {
		immediateTaskRow.TaskType = persistence.ImmediateTaskTypeExecute
	} else {
		immediateTaskRow.TaskType = persistence.ImmediateTaskTypeWaitUntil
	}

	return tx.InsertImmediateTask(ctx, immediateTaskRow)
}

func (p sqlProcessStoreImpl) publishToLocalQueue(
	ctx context.Context, tx extensions.SQLTransaction, processExecutionId uuid.UUID, messages []xdbapi.LocalQueueMessage) error {
	for _, message := range messages {
		dedupId := uuid.ParseUUID(message.GetDedupId())
		if dedupId == nil {
			dedupId = uuid.MustNewUUID()
		}

		// insert a row into xdb_sys_local_queue

		payload, err := persistence.FromEncodedObjectIntoBytes(message.Payload)
		if err != nil {
			return err
		}

		err = tx.InsertLocalQueue(ctx, extensions.LocalQueueRow{
			ProcessExecutionId: processExecutionId,
			QueueName:          message.GetQueueName(),
			DedupId:            dedupId,
			Payload:            payload,
		})
		if err != nil {
			return err
		}

		// insert a row into xdb_sys_immediate_tasks

		taskInfoBytes, err := persistence.FromImmediateTaskInfoIntoBytes(
			persistence.ImmediateTaskInfoJson{
				LocalQueueMessageInfo: &persistence.LocalQueueMessageInfoJson{
					QueueName: message.GetQueueName(),
					DedupId:   dedupId,
				},
			})
		if err != nil {
			return err
		}

		err = tx.InsertImmediateTask(ctx, extensions.ImmediateTaskRowForInsert{
			ShardId:  persistence.DefaultShardId,
			TaskType: persistence.ImmediateTaskTypeNewLocalQueueMessage,

			ProcessExecutionId: processExecutionId,
			StateId:            "",
			StateIdSequence:    0,
			Info:               taskInfoBytes,
		})
		if err != nil {
			return err
		}
	}

	return nil
}
