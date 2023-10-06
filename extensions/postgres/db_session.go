// Apache License 2.0

// Copyright (c) XDBLab organization

// Licensed to the Apache Software Foundation (ASF) under one
// or more contributor license agreements.  See the NOTICE file
// distributed with this work for additional information
// regarding copyright ownership.  The ASF licenses this file
// to you under the Apache License, Version 2.0 (the
// "License"); you may not use this file except in compliance
// with the License.  You may obtain a copy of the License at
//
//   http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing,
// software distributed under the License is distributed on an
// "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY
// KIND, either express or implied.  See the License for the
// specific language governing permissions and limitations
// under the License.    

package postgres

import (
	"context"
	"github.com/jmoiron/sqlx"
	"github.com/xdblab/xdb/extensions"
)

type dbSession struct {
	db *sqlx.DB
}

type dbTx struct {
	tx *sqlx.Tx
}

var _ extensions.SQLDBSession = (*dbSession)(nil)
var _ extensions.SQLTransaction = (*dbTx)(nil)

func newDBSession(db *sqlx.DB) *dbSession {
	return &dbSession{
		db: db,
	}
}

func (d dbSession) StartTransaction(ctx context.Context) (extensions.SQLTransaction, error) {
	tx, err := d.db.Beginx()
	if err != nil {
		return nil, err
	}
	return dbTx{
		tx: tx,
	}, nil
}

func (d dbSession) Close() error {
	return d.db.Close()
}

func (d dbTx) Commit() error {
	return d.tx.Commit()
}

func (d dbTx) Rollback() error {
	return d.tx.Rollback()
}
