package persistence

import (
	"context"
	"github.com/xdblab/xdb-apis/goapi/xdbapi"
)

// ProcessORM is for operating on the database for process execution
type ProcessORM interface {
	StartProcess(ctx context.Context, request xdbapi.ProcessExecutionStartRequest) (
		resp *xdbapi.ProcessExecutionStartResponse, alreadyStarted bool, err error)
	DescribeLatestProcess(ctx context.Context, request xdbapi.ProcessExecutionDescribeRequest) (
		resp *xdbapi.ProcessExecutionDescribeResponse, notExists bool, err error)
	Close() error
}

// ProcessMQ is consuming/processing events of process execution
type ProcessMQ interface {
	Start() error
	Stop() error
}
