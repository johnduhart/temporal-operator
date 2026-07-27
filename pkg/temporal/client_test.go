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

package temporal

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"testing"
	"time"

	"github.com/alexandrevilain/temporal-operator/api/v1beta1"
	"github.com/alexandrevilain/temporal-operator/internal/resource/mtls/certmanager"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

// newTLSSecretData returns valid secret data for GetTlSConfigFromSecret,
// holding a self-signed certificate whose CommonName is the provided name.
func newTLSSecretData(t *testing.T, commonName string) map[string][]byte {
	t.Helper()

	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)

	template := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: commonName},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(time.Hour),
		IsCA:                  true,
		BasicConstraintsValid: true,
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageDigitalSignature,
	}

	certDER, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	require.NoError(t, err)

	keyDER, err := x509.MarshalECPrivateKey(key)
	require.NoError(t, err)

	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: certDER})
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER})

	return map[string][]byte{
		certmanager.TLSCA:   certPEM,
		certmanager.TLSCert: certPEM,
		certmanager.TLSKey:  keyPEM,
	}
}

func newTLSSecret(t *testing.T, name string) *corev1.Secret {
	t.Helper()

	return &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: "default",
		},
		Data: newTLSSecretData(t, name),
	}
}

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

// clientCertificateCommonName returns the CommonName of the client certificate
// held in the provided tls config, proving which secret it was loaded from.
func clientCertificateCommonName(t *testing.T, cfg *tls.Config) string {
	t.Helper()

	require.Len(t, cfg.Certificates, 1)
	require.NotEmpty(t, cfg.Certificates[0].Certificate)

	cert, err := x509.ParseCertificate(cfg.Certificates[0].Certificate[0])
	require.NoError(t, err)

	return cert.Subject.CommonName
}

func TestBuildClusterClientOptions(t *testing.T) {
	frontendAddress := "fake-frontend.default:7233"
	internalFrontendAddress := "fake-internal-frontend-headless.default:7236"

	tests := map[string]struct {
		frontendMTLS       bool
		internodeMTLS      bool
		internalFrontend   bool
		expectedHostPort   string
		expectedTLS        bool
		expectedServerName string
		expectedCertName   string
	}{
		"no mTLS, no internal frontend": {
			expectedHostPort: frontendAddress,
		},
		"frontend mTLS, no internal frontend": {
			frontendMTLS:       true,
			expectedHostPort:   frontendAddress,
			expectedTLS:        true,
			expectedServerName: "fake-frontend.default.svc.cluster.local",
			expectedCertName:   "fake-frontend-certificate",
		},
		"internode mTLS, no internal frontend": {
			internodeMTLS:    true,
			expectedHostPort: frontendAddress,
		},
		"frontend and internode mTLS, no internal frontend": {
			frontendMTLS:       true,
			internodeMTLS:      true,
			expectedHostPort:   frontendAddress,
			expectedTLS:        true,
			expectedServerName: "fake-frontend.default.svc.cluster.local",
			expectedCertName:   "fake-frontend-certificate",
		},
		"no mTLS, internal frontend": {
			internalFrontend: true,
			expectedHostPort: internalFrontendAddress,
		},
		"frontend mTLS, internal frontend": {
			// The internal frontend serves plaintext when internode mTLS
			// is disabled, even if frontend mTLS is enabled.
			frontendMTLS:     true,
			internalFrontend: true,
			expectedHostPort: internalFrontendAddress,
		},
		"internode mTLS, internal frontend": {
			internodeMTLS:      true,
			internalFrontend:   true,
			expectedHostPort:   internalFrontendAddress,
			expectedTLS:        true,
			expectedServerName: "fake-internode.default.svc.cluster.local",
			expectedCertName:   "fake-internode-certificate",
		},
		"frontend and internode mTLS, internal frontend": {
			frontendMTLS:       true,
			internodeMTLS:      true,
			internalFrontend:   true,
			expectedHostPort:   internalFrontendAddress,
			expectedTLS:        true,
			expectedServerName: "fake-internode.default.svc.cluster.local",
			expectedCertName:   "fake-internode-certificate",
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			ctx := context.Background()
			cluster := newFakeTemporalCluster(test.frontendMTLS, test.internodeMTLS, test.internalFrontend)

			fakeClient := fake.NewClientBuilder().
				WithObjects(
					newTLSSecret(t, "fake-frontend-certificate"),
					newTLSSecret(t, "fake-internode-certificate"),
				).
				Build()

			opts, err := buildClusterClientOptions(ctx, fakeClient, cluster)
			require.NoError(t, err)

			assert.Equal(t, test.expectedHostPort, opts.HostPort)

			if !test.expectedTLS {
				assert.Nil(t, opts.ConnectionOptions.TLS)
				return
			}

			require.NotNil(t, opts.ConnectionOptions.TLS)
			assert.Equal(t, test.expectedServerName, opts.ConnectionOptions.TLS.ServerName)
			assert.NotNil(t, opts.ConnectionOptions.TLS.RootCAs)
			assert.Equal(t, test.expectedCertName, clientCertificateCommonName(t, opts.ConnectionOptions.TLS))
		})
	}
}

func TestBuildClusterClientOptionsErrors(t *testing.T) {
	tests := map[string]struct {
		frontendMTLS     bool
		internodeMTLS    bool
		internalFrontend bool
		secrets          func(t *testing.T) []client.Object
		expectedError    string
	}{
		"frontend certificate secret not found": {
			frontendMTLS: true,
			secrets: func(*testing.T) []client.Object {
				return nil
			},
			expectedError: "fake-frontend-certificate",
		},
		"internode certificate secret not found": {
			internodeMTLS:    true,
			internalFrontend: true,
			secrets: func(*testing.T) []client.Object {
				return nil
			},
			expectedError: "fake-internode-certificate",
		},
		"secret misses ca.crt": {
			frontendMTLS: true,
			secrets: func(t *testing.T) []client.Object {
				t.Helper()
				secret := newTLSSecret(t, "fake-frontend-certificate")
				delete(secret.Data, certmanager.TLSCA)
				return []client.Object{secret}
			},
			expectedError: "can't get ca.crt from client secret",
		},
		"secret misses tls.crt": {
			internodeMTLS:    true,
			internalFrontend: true,
			secrets: func(t *testing.T) []client.Object {
				t.Helper()
				secret := newTLSSecret(t, "fake-internode-certificate")
				delete(secret.Data, certmanager.TLSCert)
				return []client.Object{secret}
			},
			expectedError: "can't get tls.crt from client secret",
		},
		"secret misses tls.key": {
			internodeMTLS:    true,
			internalFrontend: true,
			secrets: func(t *testing.T) []client.Object {
				t.Helper()
				secret := newTLSSecret(t, "fake-internode-certificate")
				delete(secret.Data, certmanager.TLSKey)
				return []client.Object{secret}
			},
			expectedError: "can't get tls.key from client secret",
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			ctx := context.Background()
			cluster := newFakeTemporalCluster(test.frontendMTLS, test.internodeMTLS, test.internalFrontend)

			fakeClient := fake.NewClientBuilder().
				WithObjects(test.secrets(t)...).
				Build()

			_, err := buildClusterClientOptions(ctx, fakeClient, cluster)
			require.Error(t, err)
			assert.ErrorContains(t, err, test.expectedError)
		})
	}
}

func TestBuildClusterClientOptionsOverrides(t *testing.T) {
	ctx := context.Background()
	cluster := newFakeTemporalCluster(true, true, true)

	fakeClient := fake.NewClientBuilder().
		WithObjects(
			newTLSSecret(t, "fake-frontend-certificate"),
			newTLSSecret(t, "fake-internode-certificate"),
		).
		Build()

	overrideTLSConfig := &tls.Config{ServerName: "override.example.com", MinVersion: tls.VersionTLS12}

	opts, err := buildClusterClientOptions(ctx, fakeClient, cluster,
		WithHostPort("override.example.com:7233"),
		WithTLSConfig(overrideTLSConfig),
	)
	require.NoError(t, err)

	assert.Equal(t, "override.example.com:7233", opts.HostPort)
	assert.Same(t, overrideTLSConfig, opts.ConnectionOptions.TLS)
}
