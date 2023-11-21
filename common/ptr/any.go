// Copyright (c) 2023 xCherryIO Organization
// SPDX-License-Identifier: BUSL-1.1

package ptr

func Any[T any](obj T) *T {
	return &obj
}
