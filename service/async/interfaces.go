// Copyright (c) 2023 xCherryIO Organization
// SPDX-License-Identifier: Apache-2.0

package async

import (
	"context"
	"github.com/xcherryio/apis/goapi/xcapi"
	"github.com/xcherryio/xcherry/persistence"
)

type Server interface {
	// Start will start running on the background
	Start() error
	Stop(ctx context.Context) error
	CreateQueues(shardId int32, processStore persistence.ProcessStore)
	SetAdvertiseToClientAddressMap(advertiseToClientAddressMap map[string]string)
	GetServerAddress() string
	GetAdvertiseAddress() string
	GetAdvertiseAddressFor(shardId int32) string
}

type Service interface {
	Start() error
	NotifyPollingImmediateTask(req xcapi.NotifyImmediateTasksRequest) error
	NotifyPollingTimerTask(req xcapi.NotifyTimerTasksRequest) error
	Stop(ctx context.Context) error
	CreateQueues(shardId int32, processStore persistence.ProcessStore)
	SetAdvertiseToClientAddressMap(advertiseToClientAddressMap map[string]string)
	GetServerAddress() string
	GetAdvertiseAddress() string
	GetAdvertiseAddressFor(shardId int32) string
}
