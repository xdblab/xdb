// Copyright (c) 2023 xCherryIO Organization
// SPDX-License-Identifier: Apache-2.0

package sql

import (
	"context"
	"github.com/xcherryio/xcherry/common/log"
	"github.com/xcherryio/xcherry/config"
	"github.com/xcherryio/xcherry/extensions"
	"github.com/xcherryio/xcherry/persistence"
	"github.com/xcherryio/xcherry/persistence/data_models"
)

type sqlVisibilityStoreImpl struct {
	session extensions.SQLDBSession
	logger  log.Logger
}

func NewSQLProcessStore(sqlConfig config.SQL, logger log.Logger) (persistence.VisibilityStore, error) {
	// TODO: add visibility sql config to allow using different database for visibility
	session, err := extensions.NewSQLSession(&sqlConfig)
	return &sqlVisibilityStoreImpl{
		session: session,
		logger:  logger,
	}, err
}

func (p sqlVisibilityStoreImpl) Close() error {
	return p.session.Close()
}

func (p sqlVisibilityStoreImpl) RecordProcessExecutionStatus(
	ctx context.Context, req data_models.RecordProcessExecutionStatusRequest) error {
	// TODO: add implementation
	return nil
}
