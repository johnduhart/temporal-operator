// Licensed to Alexandre VILAIN under one or more contributor
// license agreements. See the NOTICE file distributed with
// this work for additional information regarding copyright
// ownership. Alexandre VILAIN licenses this file to you under
// the Apache License, Version 2.0 (the "License"); you may
// not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing,
// software distributed under the License is distributed on an
// "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY
// KIND, either express or implied.  See the License for the
// specific language governing permissions and limitations
// under the License.

package v1beta1_test

import (
	"testing"

	"github.com/alexandrevilain/temporal-operator/api/v1beta1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	apimeta "k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type reconcileConditionsAccessor struct {
	setReconcileError   func(status metav1.ConditionStatus, reason, message string)
	setReconcileSuccess func(status metav1.ConditionStatus, reason, message string)
	setGeneration       func(generation int64)
	conditions          func() []metav1.Condition
}

func reconcileConditionsAccessors() map[string]reconcileConditionsAccessor {
	cluster := &v1beta1.TemporalCluster{}
	namespace := &v1beta1.TemporalNamespace{}
	schedule := &v1beta1.TemporalSchedule{}

	return map[string]reconcileConditionsAccessor{
		"temporal cluster": {
			setReconcileError: func(status metav1.ConditionStatus, reason, message string) {
				v1beta1.SetTemporalClusterReconcileError(cluster, status, reason, message)
			},
			setReconcileSuccess: func(status metav1.ConditionStatus, reason, message string) {
				v1beta1.SetTemporalClusterReconcileSuccess(cluster, status, reason, message)
			},
			setGeneration: func(generation int64) { cluster.Generation = generation },
			conditions:    func() []metav1.Condition { return cluster.Status.Conditions },
		},
		"temporal namespace": {
			setReconcileError: func(status metav1.ConditionStatus, reason, message string) {
				v1beta1.SetTemporalNamespaceReconcileError(namespace, status, reason, message)
			},
			setReconcileSuccess: func(status metav1.ConditionStatus, reason, message string) {
				v1beta1.SetTemporalNamespaceReconcileSuccess(namespace, status, reason, message)
			},
			setGeneration: func(generation int64) { namespace.Generation = generation },
			conditions:    func() []metav1.Condition { return namespace.Status.Conditions },
		},
		"temporal schedule": {
			setReconcileError: func(status metav1.ConditionStatus, reason, message string) {
				v1beta1.SetTemporalScheduleReconcileError(schedule, status, reason, message)
			},
			setReconcileSuccess: func(status metav1.ConditionStatus, reason, message string) {
				v1beta1.SetTemporalScheduleReconcileSuccess(schedule, status, reason, message)
			},
			setGeneration: func(generation int64) { schedule.Generation = generation },
			conditions:    func() []metav1.Condition { return schedule.Status.Conditions },
		},
	}
}

func TestSetReconcileSuccessClearsReconcileError(t *testing.T) {
	for name, accessor := range reconcileConditionsAccessors() {
		t.Run(name, func(t *testing.T) {
			accessor.setGeneration(1)
			accessor.setReconcileError(metav1.ConditionTrue, v1beta1.ReconcileErrorReason, "the object has been modified")

			accessor.setGeneration(2)
			accessor.setReconcileSuccess(metav1.ConditionTrue, v1beta1.ReconcileSuccessReason, "")

			successCondition := apimeta.FindStatusCondition(accessor.conditions(), v1beta1.ReconcileSuccessCondition)
			require.NotNil(t, successCondition)
			assert.Equal(t, metav1.ConditionTrue, successCondition.Status)
			assert.Equal(t, v1beta1.ReconcileSuccessReason, successCondition.Reason)
			assert.EqualValues(t, 2, successCondition.ObservedGeneration)

			errorCondition := apimeta.FindStatusCondition(accessor.conditions(), v1beta1.ReconcileErrorCondition)
			require.NotNil(t, errorCondition, "ReconcileError condition should be kept, not deleted")
			assert.Equal(t, metav1.ConditionFalse, errorCondition.Status)
			assert.Equal(t, v1beta1.ReconcileSuccessReason, errorCondition.Reason)
			assert.Empty(t, errorCondition.Message)
			assert.EqualValues(t, 2, errorCondition.ObservedGeneration)
			assert.False(t, errorCondition.LastTransitionTime.IsZero())

			// A second successful reconcile should not bump the transition time
			// of an already false ReconcileError condition.
			lastTransitionTime := errorCondition.LastTransitionTime
			accessor.setReconcileSuccess(metav1.ConditionTrue, v1beta1.ReconcileSuccessReason, "")
			errorCondition = apimeta.FindStatusCondition(accessor.conditions(), v1beta1.ReconcileErrorCondition)
			require.NotNil(t, errorCondition)
			assert.Equal(t, metav1.ConditionFalse, errorCondition.Status)
			assert.Equal(t, lastTransitionTime, errorCondition.LastTransitionTime)
		})
	}
}

func TestSetReconcileSuccessReportsReconcileErrorFalseWhenUnset(t *testing.T) {
	for name, accessor := range reconcileConditionsAccessors() {
		t.Run(name, func(t *testing.T) {
			accessor.setGeneration(1)
			accessor.setReconcileSuccess(metav1.ConditionTrue, v1beta1.ReconcileSuccessReason, "")

			errorCondition := apimeta.FindStatusCondition(accessor.conditions(), v1beta1.ReconcileErrorCondition)
			require.NotNil(t, errorCondition)
			assert.Equal(t, metav1.ConditionFalse, errorCondition.Status)
			assert.Equal(t, v1beta1.ReconcileSuccessReason, errorCondition.Reason)
			assert.EqualValues(t, 1, errorCondition.ObservedGeneration)
		})
	}
}
