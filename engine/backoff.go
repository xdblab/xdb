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
	"github.com/xdblab/xdb-apis/goapi/xdbapi"
	"math"
	"time"
)

func GetNextBackoff(completedAttempts int32, firstAttemptStartTimestampSeconds int64, policy *xdbapi.RetryPolicy) (nextBackoffSeconds int32, shouldRetry bool) {
	policy = setDefaultRetryPolicyValue(policy)
	if *policy.MaximumAttempts > 0 && completedAttempts >= *policy.MaximumAttempts {
		return 0, false
	}
	nowSeconds := int64(time.Now().Unix())
	if *policy.MaximumAttemptsDurationSeconds > 0 && firstAttemptStartTimestampSeconds+int64(*policy.MaximumAttemptsDurationSeconds) < nowSeconds {
		return 0, false
	}
	initInterval := *policy.InitialIntervalSeconds
	nextInterval := int32(float64(initInterval) * math.Pow(float64(*policy.BackoffCoefficient), float64(completedAttempts)))
	if nextInterval > *policy.MaximumIntervalSeconds {
		nextInterval = *policy.MaximumIntervalSeconds
	}
	return nextInterval, true
}

func setDefaultRetryPolicyValue(policy *xdbapi.RetryPolicy) *xdbapi.RetryPolicy {
	if policy == nil {
		policy = &xdbapi.RetryPolicy{}
	}
	if policy.InitialIntervalSeconds == nil {
		policy.InitialIntervalSeconds = defaultWorkerTaskBackoffRetryPolicy.InitialIntervalSeconds
	}
	if policy.BackoffCoefficient == nil {
		policy.BackoffCoefficient = defaultWorkerTaskBackoffRetryPolicy.BackoffCoefficient
	}
	if policy.MaximumIntervalSeconds == nil {
		policy.MaximumIntervalSeconds = defaultWorkerTaskBackoffRetryPolicy.MaximumIntervalSeconds
	}
	if policy.MaximumAttempts == nil {
		policy.MaximumAttempts = defaultWorkerTaskBackoffRetryPolicy.MaximumAttempts
	}
	if policy.MaximumAttemptsDurationSeconds == nil {
		policy.MaximumAttemptsDurationSeconds = defaultWorkerTaskBackoffRetryPolicy.MaximumAttemptsDurationSeconds
	}
	return policy
}
