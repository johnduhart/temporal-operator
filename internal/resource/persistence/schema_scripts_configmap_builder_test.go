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

package persistence

import (
	"testing"

	"github.com/alexandrevilain/temporal-operator/api/v1beta1"
	"github.com/alexandrevilain/temporal-operator/pkg/version"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestShellQuote(t *testing.T) {
	assert.Equal(t, `'foo'`, shellQuote("foo"))
	assert.Equal(t, `'a b'`, shellQuote("a b"))
	// embedded single quote is escaped
	assert.Equal(t, `'it'\''s'`, shellQuote("it's"))
}

// TestGetSQLArgs_PasswordCommand asserts that a datastore using an external
// passwordCommand renders the schema tool --password flag as a shell command
// substitution (resolved at runtime by the generated setup script), instead of
// referencing a password environment variable.
func TestGetSQLArgs_PasswordCommand(t *testing.T) {
	b := &SchemaScriptsConfigmapBuilder{}

	spec := &v1beta1.DatastoreSpec{
		Name: "default",
		SQL: &v1beta1.SQLSpec{
			User:         "temporal",
			PluginName:   "postgres12",
			DatabaseName: "temporal",
			ConnectAddr:  "postgres:5432",
			PasswordCommand: &v1beta1.SQLPasswordCommandSpec{
				Command: "/bin/sh",
				Args:    []string{"-c", "printf %s test"},
			},
		},
	}

	args, err := b.getSQLArgs(spec)
	require.NoError(t, err)

	rendered := b.argsMapToString(args)
	assert.Contains(t, rendered, `--password="$('/bin/sh' '-c' 'printf %s test')"`,
		"passwordCommand must render as a shell command substitution")
	assert.NotContains(t, rendered, "PASSWORD",
		"passwordCommand must not reference a password env var")
}

// TestGetSQLArgs_PasswordSecretRef keeps asserting the classic secret-based path
// still renders a password env var reference.
func TestGetSQLArgs_PasswordSecretRef(t *testing.T) {
	b := &SchemaScriptsConfigmapBuilder{}

	spec := &v1beta1.DatastoreSpec{
		Name: "default",
		SQL: &v1beta1.SQLSpec{
			User:         "temporal",
			PluginName:   "postgres12",
			DatabaseName: "temporal",
			ConnectAddr:  "postgres:5432",
		},
		PasswordSecretRef: &v1beta1.SecretKeyReference{Name: "postgres-password", Key: "PASSWORD"},
	}

	args, err := b.getSQLArgs(spec)
	require.NoError(t, err)

	rendered := b.argsMapToString(args)
	assert.Contains(t, rendered, "--password=\"$"+spec.GetPasswordEnvVarName()+"\"")
}

func esVisibilityStore() *v1beta1.DatastoreSpec {
	return &v1beta1.DatastoreSpec{
		Name: "visibility",
		Elasticsearch: &v1beta1.ElasticsearchSpec{
			URL:      "http://elasticsearch:9200",
			Username: "elastic",
			Indices:  v1beta1.ElasticsearchIndices{Visibility: "temporal_visibility_v1_dev"},
		},
		PasswordSecretRef: &v1beta1.SecretKeyReference{Name: "es-password", Key: "PASSWORD"},
	}
}

func esBuilder(v string) *SchemaScriptsConfigmapBuilder {
	return &SchemaScriptsConfigmapBuilder{
		instance: &v1beta1.TemporalCluster{
			Spec: v1beta1.TemporalClusterSpec{
				Version: version.MustNewVersionFromString(v),
			},
		},
	}
}

// TestESVisibility_Tool_Post130 asserts that on Temporal >= 1.30 the ES visibility
// setup/update scripts drive temporal-elasticsearch-tool (curl/jq were removed from
// the admin-tools image) instead of curl.
func TestESVisibility_Tool_Post130(t *testing.T) {
	b := esBuilder("1.30.5")
	store := esVisibilityStore()

	setup, err := b.GetStoreSetupTemplate(store)
	require.NoError(t, err)
	assert.Contains(t, setup, "temporal-elasticsearch-tool")
	assert.Contains(t, setup, "setup-schema")
	assert.Contains(t, setup, `create-index --index "temporal_visibility_v1_dev"`)
	assert.Contains(t, setup, `--endpoint="http://elasticsearch:9200"`)
	assert.Contains(t, setup, `--user="elastic"`)
	assert.Contains(t, setup, "--password=\"$"+store.GetPasswordEnvVarName()+"\"")
	assert.NotContains(t, setup, "curl")

	update, err := b.GetStoreUpdateTemplate(store, VisibilitySchema)
	require.NoError(t, err)
	assert.Contains(t, update, "temporal-elasticsearch-tool")
	assert.Contains(t, update, `update-schema --index "temporal_visibility_v1_dev"`)
	assert.NotContains(t, update, "curl")
}

// TestESVisibility_Curl_Pre130 asserts that on Temporal < 1.30 the legacy curl-based
// scripts are still generated (older admin-tools images ship curl and lack the tool).
func TestESVisibility_Curl_Pre130(t *testing.T) {
	b := esBuilder("1.29.7")
	store := esVisibilityStore()

	setup, err := b.GetStoreSetupTemplate(store)
	require.NoError(t, err)
	assert.Contains(t, setup, "curl")
	assert.Contains(t, setup, "_template")
	assert.NotContains(t, setup, "temporal-elasticsearch-tool")
}
