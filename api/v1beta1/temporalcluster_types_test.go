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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"
)

func newFakeTemporalCluster(frontendMTLS, internodeMTLS, internalFrontend bool) *v1beta1.TemporalCluster {
	cluster := &v1beta1.TemporalCluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "fake",
			Namespace: "default",
		},
		Spec: v1beta1.TemporalClusterSpec{
			Services: &v1beta1.ServicesSpec{
				Frontend: &v1beta1.ServiceSpec{
					Port: ptr.To[int32](7233),
				},
			},
		},
	}

	if internalFrontend {
		cluster.Spec.Services.InternalFrontend = &v1beta1.InternalFrontendServiceSpec{
			ServiceSpec: v1beta1.ServiceSpec{
				Port: ptr.To[int32](7236),
			},
			Enabled: true,
		}
	}

	if frontendMTLS || internodeMTLS {
		cluster.Spec.MTLS = &v1beta1.MTLSSpec{
			Provider: v1beta1.CertManagerMTLSProvider,
		}
		if frontendMTLS {
			cluster.Spec.MTLS.Frontend = &v1beta1.FrontendMTLSSpec{Enabled: true}
		}
		if internodeMTLS {
			cluster.Spec.MTLS.Internode = &v1beta1.InternodeMTLSSpec{Enabled: true}
		}
	}

	return cluster
}

func TestGetPublicClientAddressAndGetOperatorClientAddress(t *testing.T) {
	frontendAddress := "fake-frontend.default:7233"
	internalFrontendAddress := "fake-internal-frontend-headless.default:7236"

	tests := map[string]struct {
		frontendMTLS            bool
		internodeMTLS           bool
		internalFrontend        bool
		expectedPublicAddress   string
		expectedOperatorAddress string
	}{
		"no mTLS, no internal frontend": {
			expectedPublicAddress:   frontendAddress,
			expectedOperatorAddress: frontendAddress,
		},
		"frontend mTLS, no internal frontend": {
			frontendMTLS:            true,
			expectedPublicAddress:   frontendAddress,
			expectedOperatorAddress: frontendAddress,
		},
		"internode mTLS, no internal frontend": {
			internodeMTLS:           true,
			expectedPublicAddress:   frontendAddress,
			expectedOperatorAddress: frontendAddress,
		},
		"frontend and internode mTLS, no internal frontend": {
			frontendMTLS:            true,
			internodeMTLS:           true,
			expectedPublicAddress:   frontendAddress,
			expectedOperatorAddress: frontendAddress,
		},
		"no mTLS, internal frontend": {
			internalFrontend:        true,
			expectedPublicAddress:   internalFrontendAddress,
			expectedOperatorAddress: internalFrontendAddress,
		},
		"frontend mTLS, internal frontend": {
			// GetPublicClientAddress always returns the public frontend address
			// when frontend mTLS is enabled, while the operator uses the
			// internal frontend for its own connections.
			frontendMTLS:            true,
			internalFrontend:        true,
			expectedPublicAddress:   frontendAddress,
			expectedOperatorAddress: internalFrontendAddress,
		},
		"internode mTLS, internal frontend": {
			internodeMTLS:           true,
			internalFrontend:        true,
			expectedPublicAddress:   internalFrontendAddress,
			expectedOperatorAddress: internalFrontendAddress,
		},
		"frontend and internode mTLS, internal frontend": {
			frontendMTLS:            true,
			internodeMTLS:           true,
			internalFrontend:        true,
			expectedPublicAddress:   frontendAddress,
			expectedOperatorAddress: internalFrontendAddress,
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			cluster := newFakeTemporalCluster(test.frontendMTLS, test.internodeMTLS, test.internalFrontend)

			assert.Equal(t, test.expectedPublicAddress, cluster.GetPublicClientAddress())
			assert.Equal(t, test.expectedOperatorAddress, cluster.GetOperatorClientAddress())
		})
	}
}
