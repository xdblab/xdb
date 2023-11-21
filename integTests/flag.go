// Copyright (c) 2023 xCherryIO Organization
// SPDX-License-Identifier: BUSL-1.1

package integTests

import "flag"

var useLocalServer = flag.Bool("useLocalServer", false,
	"run integ test against local server")

var createServerWithPostgres = flag.Bool("createServerWithPostgres", true,
	"when not useLocalServer, create a server with postgres and run integ test against ")
