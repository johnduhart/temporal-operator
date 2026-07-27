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

package webhooks_test

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"testing"

	"github.com/alexandrevilain/temporal-operator/api/v1beta1"
	"github.com/alexandrevilain/temporal-operator/internal/discovery"
	"github.com/alexandrevilain/temporal-operator/webhooks"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// collectNullJSONPaths recursively walks an unmarshaled JSON document and
// collects the paths of all null values found.
func collectNullJSONPaths(path string, value interface{}, nulls *[]string) {
	switch typedValue := value.(type) {
	case nil:
		*nulls = append(*nulls, path)
	case map[string]interface{}:
		for key, child := range typedValue {
			collectNullJSONPaths(fmt.Sprintf("%s.%s", path, key), child, nulls)
		}
	case []interface{}:
		for i, child := range typedValue {
			collectNullJSONPaths(fmt.Sprintf("%s[%d]", path, i), child, nulls)
		}
	}
}

// TestDefaultDoesNotSerializeNullValues ensures that the object returned by the
// defaulting webhook never serializes JSON null values. The api-server validates
// the mutating webhook's response against the CRD structural schema, which
// rejects null for non-nullable fields. Nil pointers, slices and maps whose
// json tag lacks omitempty would be serialized as null and make the api-server
// reject an otherwise valid TemporalCluster.
func TestDefaultDoesNotSerializeNullValues(t *testing.T) {
	wh := &webhooks.TemporalClusterWebhook{
		AvailableAPIs: &discovery.AvailableAPIs{},
	}

	// A minimal valid TemporalCluster as a user would write it:
	// authorization is set with authorizer and claimMapper but jwtKeyProvider
	// is omitted, and every optional pointer field is left nil.
	cluster := &v1beta1.TemporalCluster{
		TypeMeta: v1beta1.TemporalClusterTypeMeta,
		ObjectMeta: metav1.ObjectMeta{
			Name:      "fake",
			Namespace: "default",
		},
		Spec: v1beta1.TemporalClusterSpec{
			NumHistoryShards: 1,
			Persistence: v1beta1.TemporalPersistenceSpec{
				DefaultStore: &v1beta1.DatastoreSpec{
					SQL: &v1beta1.SQLSpec{
						User:         "temporal",
						PluginName:   "postgres",
						DatabaseName: "temporal",
						ConnectAddr:  "postgres:5432",
					},
					PasswordSecretRef: &v1beta1.SecretKeyReference{
						Name: "postgres-password",
					},
				},
				VisibilityStore: &v1beta1.DatastoreSpec{
					SQL: &v1beta1.SQLSpec{
						User:         "temporal",
						PluginName:   "postgres",
						DatabaseName: "temporal_visibility",
						ConnectAddr:  "postgres:5432",
					},
					PasswordSecretRef: &v1beta1.SecretKeyReference{
						Name: "postgres-password",
					},
				},
			},
			Authorization: &v1beta1.AuthorizationSpec{
				Authorizer:  "default",
				ClaimMapper: "default",
			},
		},
	}

	err := wh.Default(context.Background(), cluster)
	require.NoError(t, err)

	data, err := json.Marshal(cluster.Spec)
	require.NoError(t, err)

	var decoded interface{}
	err = json.Unmarshal(data, &decoded)
	require.NoError(t, err)

	nulls := []string{}
	collectNullJSONPaths("spec", decoded, &nulls)
	sort.Strings(nulls)

	assert.Empty(t, nulls, "defaulted TemporalCluster spec serializes JSON null values, which the CRD structural schema rejects")
}
