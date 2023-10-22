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

package engine

import (
	"context"
	"fmt"
	"io/ioutil"
	"net/http"
	"time"

	"github.com/xdblab/xdb-apis/goapi/xdbapi"
	"github.com/xdblab/xdb/common/log"
	"github.com/xdblab/xdb/common/log/tag"
	"github.com/xdblab/xdb/common/ptr"
	"github.com/xdblab/xdb/common/urlautofix"
	"github.com/xdblab/xdb/config"
	"github.com/xdblab/xdb/persistence"
)

type immediateTaskConcurrentProcessor struct {
	rootCtx           context.Context
	cfg               config.Config
	taskToProcessChan chan persistence.ImmediateTask
	// for quickly checking if the shardId is being processed
	currentShards map[int32]bool
	// shardId to the channel
	taskToCommitChans map[int32]chan<- persistence.ImmediateTask
	taskNotifier      TaskNotifier
	store             persistence.ProcessStore
	logger            log.Logger
}

func NewImmediateTaskConcurrentProcessor(
	ctx context.Context, cfg config.Config, notifier TaskNotifier,
	store persistence.ProcessStore, logger log.Logger,
) ImmediateTaskProcessor {
	bufferSize := cfg.AsyncService.ImmediateTaskQueue.ProcessorBufferSize
	return &immediateTaskConcurrentProcessor{
		rootCtx:           ctx,
		cfg:               cfg,
		taskToProcessChan: make(chan persistence.ImmediateTask, bufferSize),
		currentShards:     map[int32]bool{},
		taskToCommitChans: make(map[int32]chan<- persistence.ImmediateTask),
		taskNotifier:      notifier,
		store:             store,
		logger:            logger,
	}
}

func (w *immediateTaskConcurrentProcessor) Stop(context.Context) error {
	return nil
}
func (w *immediateTaskConcurrentProcessor) GetTasksToProcessChan() chan<- persistence.ImmediateTask {
	return w.taskToProcessChan
}

func (w *immediateTaskConcurrentProcessor) AddImmediateTaskQueue(
	shardId int32, tasksToCommitChan chan<- persistence.ImmediateTask,
) (alreadyExisted bool) {
	exists := w.currentShards[shardId]
	w.currentShards[shardId] = true
	w.taskToCommitChans[shardId] = tasksToCommitChan
	return exists
}

func (w *immediateTaskConcurrentProcessor) Start() error {
	concurrency := w.cfg.AsyncService.ImmediateTaskQueue.ProcessorConcurrency

	for i := 0; i < concurrency; i++ {
		go func() {
			for {
				select {
				case <-w.rootCtx.Done():
					return
				case task, ok := <-w.taskToProcessChan:
					if !ok {
						return
					}
					if !w.currentShards[task.ShardId] {
						w.logger.Info("skip the stale task that is due to shard movement", tag.Shard(task.ShardId), tag.ID(task.GetTaskId()))
						continue
					}

					err := w.processImmediateTask(w.rootCtx, task)

					if w.currentShards[task.ShardId] { // check again
						commitChan := w.taskToCommitChans[task.ShardId]
						if err != nil {
							// put it back to the queue for immediate retry
							// Note that if the error is because of invoking worker APIs, it will be sent to
							// timer task instead
							// TODO add a counter to a task, and when exceeding certain limit, put the task into a different channel to process "slowly"
							w.logger.Info("failed to process immediate task due to internal error, put back to queue for immediate retry", tag.Error(err))
							w.taskToProcessChan <- task
						} else {
							commitChan <- task
						}
					}
				}
			}
		}()
	}
	return nil
}

func (w *immediateTaskConcurrentProcessor) processImmediateTask(
	ctx context.Context, task persistence.ImmediateTask,
) error {

	w.logger.Debug("start executing immediate task", tag.ID(task.GetTaskId()), tag.ImmediateTaskType(task.TaskType.String()))

	if task.TaskType == persistence.ImmediateTaskTypeNewLocalQueueMessage {
		return w.processLocalQueueMessageTask(ctx, task)
	}

	prep, err := w.store.PrepareStateExecution(ctx, persistence.PrepareStateExecutionRequest{
		ProcessExecutionId: task.ProcessExecutionId,
		StateExecutionId: persistence.StateExecutionId{
			StateId:         task.StateId,
			StateIdSequence: task.StateIdSequence,
		},
	})
	if err != nil {
		return err
	}

	iwfWorkerBaseUrl := urlautofix.FixWorkerUrl(prep.Info.WorkerURL)
	apiClient := xdbapi.NewAPIClient(&xdbapi.Configuration{
		Servers: []xdbapi.ServerConfiguration{
			{
				URL: iwfWorkerBaseUrl,
			},
		},
	})

	if prep.WaitUntilStatus == persistence.StateExecutionStatusRunning {
		return w.processWaitUntilTask(ctx, task, *prep, apiClient)
	} else if prep.ExecuteStatus == persistence.StateExecutionStatusRunning {
		return w.processExecuteTask(ctx, task, *prep, apiClient)
	} else {
		w.logger.Warn("noop for immediate task ",
			tag.ID(tag.AnyToStr(task.TaskSequence)),
			tag.Value(fmt.Sprintf("waitUntilStatus %v, executeStatus %v",
				prep.WaitUntilStatus, prep.ExecuteStatus)))
		return nil
	}
}

func (w *immediateTaskConcurrentProcessor) processWaitUntilTask(
	ctx context.Context, task persistence.ImmediateTask,
	prep persistence.PrepareStateExecutionResponse, apiClient *xdbapi.APIClient,
) error {

	workerApiCtx, cancF := w.createContextWithTimeout(ctx, task.TaskType, prep.Info.StateConfig)
	defer cancF()

	if task.ImmediateTaskInfo.WorkerTaskBackoffInfo == nil {
		task.ImmediateTaskInfo.WorkerTaskBackoffInfo = createWorkerTaskBackoffInfo()
	}
	task.ImmediateTaskInfo.WorkerTaskBackoffInfo.CompletedAttempts++

	req := apiClient.DefaultAPI.ApiV1XdbWorkerAsyncStateWaitUntilPost(workerApiCtx)
	resp, httpResp, err := req.AsyncStateWaitUntilRequest(
		xdbapi.AsyncStateWaitUntilRequest{
			Context:     createApiContext(prep, task),
			ProcessType: prep.Info.ProcessType,
			StateId:     task.StateId,
			StateInput: &xdbapi.EncodedObject{
				Encoding: prep.Input.Encoding,
				Data:     prep.Input.Data,
			},
		},
	).Execute()
	if httpResp != nil {
		defer httpResp.Body.Close()
	}
	if w.checkResponseAndError(err, httpResp) {
		status, details, err := w.composeHttpError(err, httpResp, prep.Info, task)

		nextIntervalSecs, shouldRetry := w.checkRetry(task, prep.Info)
		if shouldRetry {
			return w.retryTask(ctx, task, prep, nextIntervalSecs, status, details)
		}
		// TODO otherwise we should fail the state and process execution if the backoff is exhausted, unless using a recovery policy
		return err
	}

	compResp, err := w.store.ProcessWaitUntilExecution(ctx, persistence.ProcessWaitUntilExecutionRequest{
		ProcessExecutionId: task.ProcessExecutionId,
		StateExecutionId: persistence.StateExecutionId{
			StateId:         task.StateId,
			StateIdSequence: task.StateIdSequence,
		},
		Prepare:             prep,
		CommandRequest:      resp.GetCommandRequest(),
		PublishToLocalQueue: resp.GetPublishToLocalQueue(),
		TaskShardId:         task.ShardId,
	})
	if err != nil {
		return err
	}
	if compResp.HasNewImmediateTask {
		w.notifyNewImmediateTask(prep, task)
	}
	return nil
}

func createWorkerTaskBackoffInfo() *persistence.WorkerTaskBackoffInfoJson {
	return &persistence.WorkerTaskBackoffInfoJson{
		CompletedAttempts:            int32(0),
		FirstAttemptTimestampSeconds: time.Now().Unix(),
	}
}

func createApiContext(prep persistence.PrepareStateExecutionResponse, task persistence.ImmediateTask) xdbapi.Context {
	return xdbapi.Context{
		ProcessId:          prep.Info.ProcessId,
		ProcessExecutionId: task.ProcessExecutionId.String(),
		StateExecutionId:   ptr.Any(task.StateExecutionId.GetStateExecutionId()),

		Attempt:               ptr.Any(task.ImmediateTaskInfo.WorkerTaskBackoffInfo.CompletedAttempts),
		FirstAttemptTimestamp: ptr.Any(task.ImmediateTaskInfo.WorkerTaskBackoffInfo.FirstAttemptTimestampSeconds),

		// TODO add processStartTime
	}
}

func (w *immediateTaskConcurrentProcessor) processExecuteTask(
	ctx context.Context, task persistence.ImmediateTask,
	prep persistence.PrepareStateExecutionResponse, apiClient *xdbapi.APIClient,
) error {

	if task.ImmediateTaskInfo.WorkerTaskBackoffInfo == nil {
		task.ImmediateTaskInfo.WorkerTaskBackoffInfo = createWorkerTaskBackoffInfo()
	}
	task.ImmediateTaskInfo.WorkerTaskBackoffInfo.CompletedAttempts++

	ctx, cancF := w.createContextWithTimeout(ctx, task.TaskType, prep.Info.StateConfig)
	defer cancF()

	req := apiClient.DefaultAPI.ApiV1XdbWorkerAsyncStateExecutePost(ctx)
	resp, httpResp, err := req.AsyncStateExecuteRequest(
		xdbapi.AsyncStateExecuteRequest{
			Context:     createApiContext(prep, task),
			ProcessType: prep.Info.ProcessType,
			StateId:     task.StateId,
			StateInput: &xdbapi.EncodedObject{
				Encoding: prep.Input.Encoding,
				Data:     prep.Input.Data,
			},
		},
	).Execute()
	if httpResp != nil {
		defer httpResp.Body.Close()
	}
	if err == nil {
		err = checkDecision(resp.StateDecision)
	}
	if w.checkResponseAndError(err, httpResp) {
		status, details, err := w.composeHttpError(err, httpResp, prep.Info, task)

		nextIntervalSecs, shouldRetry := w.checkRetry(task, prep.Info)
		if shouldRetry {
			return w.retryTask(ctx, task, prep, nextIntervalSecs, status, details)
		}
		// TODO otherwise we should fail the state and process execution if the backoff is exhausted(unless using a state recovery policy)
		// Also need to abort all other state executions
		return err
	}

	compResp, err := w.store.CompleteExecuteExecution(ctx, persistence.CompleteExecuteExecutionRequest{
		ProcessExecutionId: task.ProcessExecutionId,
		StateExecutionId: persistence.StateExecutionId{
			StateId:         task.StateId,
			StateIdSequence: task.StateIdSequence,
		},
		Prepare:             prep,
		StateDecision:       resp.StateDecision,
		PublishToLocalQueue: resp.GetPublishToLocalQueue(),
		TaskShardId:         task.ShardId,
	})
	if err != nil {
		return err
	}
	if compResp.HasNewImmediateTask {
		w.notifyNewImmediateTask(prep, task)
	}
	return nil
}

func (w *immediateTaskConcurrentProcessor) createContextWithTimeout(
	ctx context.Context, taskType persistence.ImmediateTaskType, stateConfig *xdbapi.AsyncStateConfig,
) (context.Context, context.CancelFunc) {
	qCfg := w.cfg.AsyncService.ImmediateTaskQueue
	timeout := qCfg.DefaultAsyncStateAPITimeout
	if stateConfig != nil {
		if taskType == persistence.ImmediateTaskTypeWaitUntil {
			if stateConfig.GetWaitUntilApiTimeoutSeconds() > 0 {
				timeout = time.Duration(stateConfig.GetWaitUntilApiTimeoutSeconds()) * time.Second
			}
		} else if taskType == persistence.ImmediateTaskTypeExecute {
			if stateConfig.GetExecuteApiTimeoutSeconds() > 0 {
				timeout = time.Duration(stateConfig.GetExecuteApiTimeoutSeconds()) * time.Second
			}
		} else {
			panic("invalid taskType " + string(taskType) + ", critical code bug")
		}
		if timeout > qCfg.MaxAsyncStateAPITimeout {
			timeout = qCfg.MaxAsyncStateAPITimeout
		}
	}
	return context.WithTimeout(ctx, timeout)
}

func (w *immediateTaskConcurrentProcessor) notifyNewImmediateTask(
	prep persistence.PrepareStateExecutionResponse, task persistence.ImmediateTask,
) {
	w.taskNotifier.NotifyNewImmediateTasks(xdbapi.NotifyImmediateTasksRequest{
		ShardId:            persistence.DefaultShardId,
		Namespace:          &prep.Info.Namespace,
		ProcessId:          &prep.Info.ProcessId,
		ProcessExecutionId: ptr.Any(task.ProcessExecutionId.String()),
	})
}

func (w *immediateTaskConcurrentProcessor) checkRetry(
	task persistence.ImmediateTask, info persistence.AsyncStateExecutionInfoJson,
) (nextBackoffSeconds int32, shouldRetry bool) {
	return GetNextBackoff(
		task.ImmediateTaskInfo.WorkerTaskBackoffInfo.CompletedAttempts,
		task.ImmediateTaskInfo.WorkerTaskBackoffInfo.FirstAttemptTimestampSeconds,
		info.StateConfig.WaitUntilApiRetryPolicy)
}

func (w *immediateTaskConcurrentProcessor) retryTask(
	ctx context.Context, task persistence.ImmediateTask,
	prep persistence.PrepareStateExecutionResponse, nextIntervalSecs int32,
	LastFailureStatus int32, LastFailureDetails string,
) error {
	fireTimeUnixSeconds := time.Now().Unix() + int64(nextIntervalSecs)
	err := w.store.BackoffImmediateTask(ctx, persistence.BackoffImmediateTaskRequest{
		LastFailureStatus:    LastFailureStatus,
		LastFailureDetails:   LastFailureDetails,
		Prep:                 prep,
		FireTimestampSeconds: fireTimeUnixSeconds,
		Task:                 task,
	})
	if err != nil {
		return err
	}
	w.taskNotifier.NotifyNewTimerTasks(xdbapi.NotifyTimerTasksRequest{
		ShardId:            persistence.DefaultShardId,
		Namespace:          &prep.Info.Namespace,
		ProcessId:          &prep.Info.ProcessId,
		ProcessExecutionId: ptr.Any(task.ProcessExecutionId.String()),
		FireTimestamps:     []int64{fireTimeUnixSeconds},
	})
	w.logger.Debug("retry is scheduled", tag.Value(nextIntervalSecs), tag.Value(time.Unix(fireTimeUnixSeconds, 0)))
	return nil
}

func checkDecision(decision xdbapi.StateDecision) error {
	if decision.HasThreadCloseDecision() && len(decision.GetNextStates()) > 0 {
		return fmt.Errorf("cannot have both thread decision and next states")
	}
	return nil
}

func (w *immediateTaskConcurrentProcessor) checkResponseAndError(err error, httpResp *http.Response) bool {
	status := 0
	if httpResp != nil {
		status = httpResp.StatusCode
	}
	w.logger.Debug("immediate task executed", tag.Error(err), tag.StatusCode(status))

	if err != nil || (httpResp != nil && httpResp.StatusCode != http.StatusOK) {
		return true
	}
	return false
}

func (w *immediateTaskConcurrentProcessor) composeHttpError(
	err error, httpResp *http.Response,
	info persistence.AsyncStateExecutionInfoJson, task persistence.ImmediateTask,
) (int32, string, error) {
	responseBody := "None"
	var statusCode int32
	if httpResp != nil {
		body, err := ioutil.ReadAll(httpResp.Body)
		if err != nil {
			responseBody = "cannot read body from http response"
		} else {
			responseBody = string(body)
		}
		statusCode = int32(httpResp.StatusCode)
	}

	details := fmt.Sprintf("errMsg: %v, responseBody: %v", err, responseBody)
	maxDetailSize := w.cfg.AsyncService.ImmediateTaskQueue.MaxStateAPIFailureDetailSize
	if len(details) > maxDetailSize {
		details = details[:maxDetailSize] + "...(truncated)"
	}

	w.logger.Info(task.TaskType.String()+" API return error",
		tag.Error(err),
		tag.StatusCode(int(statusCode)),
		tag.Namespace(info.Namespace),
		tag.ProcessType(info.ProcessType),
		tag.ProcessId(info.ProcessId),
		tag.ProcessExecutionId(task.ProcessExecutionId.String()),
		tag.StateExecutionId(task.GetStateExecutionId()),
	)

	return statusCode, details, fmt.Errorf("statusCode: %v, errMsg: %w, responseBody: %v", statusCode, err, responseBody)
}

func (w *immediateTaskConcurrentProcessor) processLocalQueueMessageTask(
	ctx context.Context, task persistence.ImmediateTask,
) error {
	return w.store.ProcessLocalQueueMessage(ctx, persistence.ProcessLocalQueueMessageRequest{
		TaskShardId:  task.ShardId,
		TaskSequence: task.GetTaskSequence(),

		ProcessExecutionId: task.ProcessExecutionId,

		QueueName: task.ImmediateTaskInfo.LocalQueueMessageInfo.QueueName,
		DedupId:   task.ImmediateTaskInfo.LocalQueueMessageInfo.DedupId,
		Payload:   task.ImmediateTaskInfo.LocalQueueMessageInfo.Payload,
	})
}
