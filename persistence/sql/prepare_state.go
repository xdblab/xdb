// Copyright (c) 2023 XDBLab Organization
// SPDX-License-Identifier: BUSL-1.1

package sql

import (
	"context"
	"github.com/xdblab/xdb-apis/goapi/xdbapi"
	"github.com/xdblab/xdb/persistence/data_models"

	"github.com/xdblab/xdb/extensions"
	"github.com/xdblab/xdb/persistence"
)

func (p sqlProcessStoreImpl) PrepareStateExecution(
	ctx context.Context, request persistence.PrepareStateExecutionRequest,
) (*persistence.PrepareStateExecutionResponse, error) {
	stateRow, err := p.session.SelectAsyncStateExecution(
		ctx, extensions.AsyncStateExecutionSelectFilter{
			ProcessExecutionId: request.ProcessExecutionId,
			StateId:            request.StateId,
			StateIdSequence:    request.StateIdSequence,
		})
	if err != nil {
		return nil, err
	}

	info, err := data_models.BytesToAsyncStateExecutionInfo(stateRow.Info)
	if err != nil {
		return nil, err
	}

	input, err := data_models.BytesToEncodedObject(stateRow.Input)
	if err != nil {
		return nil, err
	}

	commandResultsJson, err := data_models.BytesToCommandResultsJson(stateRow.WaitUntilCommandResults)
	if err != nil {
		return nil, err
	}

	commandRequest, err := data_models.BytesToCommandRequest(stateRow.WaitUntilCommands)
	if err != nil {
		return nil, err
	}

	commandResults := p.prepareWaitUntilCommandResults(commandResultsJson, commandRequest)

	return &persistence.PrepareStateExecutionResponse{
		Status:                  stateRow.Status,
		WaitUntilCommandResults: commandResults,
		PreviousVersion:         stateRow.PreviousVersion,
		Info:                    info,
		Input:                   input,
	}, nil
}

func (p sqlProcessStoreImpl) prepareWaitUntilCommandResults(
	commandResultsJson data_models.CommandResultsJson, commandRequest xdbapi.CommandRequest,
) xdbapi.CommandResults {
	commandResults := xdbapi.CommandResults{}

	for idx := range commandRequest.TimerCommands {
		timerResult := xdbapi.TimerResult{
			Status: xdbapi.WAITING_COMMAND,
		}

		fired, ok := commandResultsJson.TimerResults[idx]
		if ok {
			if fired {
				timerResult.Status = xdbapi.COMPLETED_COMMAND
			} else {
				timerResult.Status = xdbapi.SKIPPED_COMMAND
			}
		}

		commandResults.TimerResults = append(commandResults.TimerResults, timerResult)
	}

	for idx, localQueueCommand := range commandRequest.LocalQueueCommands {
		localQueueResult := xdbapi.LocalQueueResult{
			Status:    xdbapi.WAITING_COMMAND,
			QueueName: localQueueCommand.GetQueueName(),
			Messages:  nil,
		}

		result, ok := commandResultsJson.LocalQueueResults[idx]
		if ok {
			localQueueResult.Status = xdbapi.COMPLETED_COMMAND
			localQueueResult.Messages = result
		}

		commandResults.LocalQueueResults = append(commandResults.LocalQueueResults, localQueueResult)
	}

	return commandResults
}
