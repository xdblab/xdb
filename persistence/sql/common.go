// Copyright (c) 2023 XDBLab Organization
// SPDX-License-Identifier: BUSL-1.1

package sql

import (
	"github.com/xdblab/xdb/common/ptr"
	"github.com/xdblab/xdb/extensions"
	"github.com/xdblab/xdb/persistence"
	"math"
)

func createGetTimerTaskResponse(
	shardId int32, dbTimerTasks []extensions.TimerTaskRow, reqPageSize *int32,
) (*persistence.GetTimerTasksResponse, error) {
	var tasks []persistence.TimerTask
	for _, t := range dbTimerTasks {
		info, err := persistence.BytesToTimerTaskInfo(t.Info)
		if err != nil {
			return nil, err
		}
		tasks = append(tasks, persistence.TimerTask{
			ShardId:              shardId,
			FireTimestampSeconds: t.FireTimeUnixSeconds,
			TaskSequence:         ptr.Any(t.TaskSequence),

			TaskType:           t.TaskType,
			ProcessExecutionId: t.ProcessExecutionId,
			StateExecutionId: persistence.StateExecutionId{
				StateId:         t.StateId,
				StateIdSequence: t.StateIdSequence,
			},
			TimerTaskInfo: info,
		})
	}
	resp := &persistence.GetTimerTasksResponse{
		Tasks: tasks,
	}
	if len(dbTimerTasks) > 0 {
		firstTask := dbTimerTasks[0]
		lastTask := dbTimerTasks[len(dbTimerTasks)-1]
		resp.MinFireTimestampSecondsInclusive = firstTask.FireTimeUnixSeconds
		resp.MaxFireTimestampSecondsInclusive = lastTask.FireTimeUnixSeconds

		resp.MinSequenceInclusive = math.MaxInt64
		resp.MaxSequenceInclusive = math.MinInt64
		for _, t := range dbTimerTasks {
			if t.TaskSequence < resp.MinSequenceInclusive {
				resp.MinSequenceInclusive = t.TaskSequence
			}
			if t.TaskSequence > resp.MaxSequenceInclusive {
				resp.MaxSequenceInclusive = t.TaskSequence
			}
		}
	}
	if reqPageSize != nil {
		if len(dbTimerTasks) == int(*reqPageSize) {
			resp.FullPage = true
		}
	}
	return resp, nil
}
