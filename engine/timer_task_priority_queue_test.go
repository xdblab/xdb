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
	"container/heap"
	"github.com/stretchr/testify/assert"
	"github.com/xdblab/xdb/persistence"
	"testing"
)

func TestTimerTaskPriorityQueue(t *testing.T) {
	pq := NewTimerTaskPriorityQueue([]persistence.TimerTask{
		{FireTimestampSeconds: 6},
		{FireTimestampSeconds: 7},
		{FireTimestampSeconds: 5},
		{FireTimestampSeconds: 8},
	})

	heap.Init(&pq)

	heap.Push(&pq, &persistence.TimerTask{FireTimestampSeconds: 3})
	heap.Push(&pq, &persistence.TimerTask{FireTimestampSeconds: 1})
	heap.Push(&pq, &persistence.TimerTask{FireTimestampSeconds: 2})
	heap.Push(&pq, &persistence.TimerTask{FireTimestampSeconds: 4})

	for i := 0; i < 8; i++ {
		task0 := pq[0]
		task := heap.Pop(&pq)
		assert.Equal(t, task0, task)
		task1, ok := task.(*persistence.TimerTask)
		assert.Equal(t, true, ok)

		assert.Equal(t, int64(i+1), task1.FireTimestampSeconds)
	}
}
