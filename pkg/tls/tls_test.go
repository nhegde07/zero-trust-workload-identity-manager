/*
Copyright 2026 Red Hat, Inc.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package tls

import (
	"context"
	"crypto/tls"
	"reflect"
	"testing"

	configv1 "github.com/openshift/api/config/v1"
	libgocrypto "github.com/openshift/library-go/pkg/crypto"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/rest"
)

func applyTLSConfigResult(t *testing.T, result TLSConfigResult) *tls.Config {
	t.Helper()

	if result.TLSConfig == nil {
		t.Fatal("expected TLSConfig function to be set")
	}

	cfg := &tls.Config{}
	result.TLSConfig(cfg)
	return cfg
}

func TestResolveTLSConfig(t *testing.T) {
	oldProfile := configv1.TLSProfiles[configv1.TLSProfileOldType]
	defaultProfile := configv1.TLSProfiles[libgocrypto.DefaultTLSProfileType]
	modernProfile := configv1.TLSProfiles[configv1.TLSProfileModernType]

	tests := []struct {
		name                 string
		apiServer            configv1.APIServer
		wantError            bool
		wantMinTLSVersion    uint16
		wantCipherSuites     bool
		wantAdherencePolicy  configv1.TLSAdherencePolicy
		wantTLSProfileSpec   configv1.TLSProfileSpec
	}{
		{
			name: "StrictAllComponents honors cluster Old profile",
			apiServer: configv1.APIServer{
				ObjectMeta: metav1.ObjectMeta{Name: "cluster"},
				Spec: configv1.APIServerSpec{
					TLSSecurityProfile: &configv1.TLSSecurityProfile{
						Type: configv1.TLSProfileOldType,
					},
					TLSAdherence: configv1.TLSAdherencePolicyStrictAllComponents,
				},
			},
			wantMinTLSVersion:   libgocrypto.TLSVersionOrDie(string(oldProfile.MinTLSVersion)),
			wantCipherSuites:    true,
			wantAdherencePolicy: configv1.TLSAdherencePolicyStrictAllComponents,
			wantTLSProfileSpec:  *oldProfile,
		},
		{
			name: "NoOpinion uses default Intermediate profile",
			apiServer: configv1.APIServer{
				ObjectMeta: metav1.ObjectMeta{Name: "cluster"},
				Spec: configv1.APIServerSpec{
					TLSSecurityProfile: &configv1.TLSSecurityProfile{
						Type: configv1.TLSProfileOldType,
					},
					TLSAdherence: configv1.TLSAdherencePolicyNoOpinion,
				},
			},
			wantMinTLSVersion:   libgocrypto.TLSVersionOrDie(string(defaultProfile.MinTLSVersion)),
			wantCipherSuites:    true,
			wantAdherencePolicy: configv1.TLSAdherencePolicyNoOpinion,
			wantTLSProfileSpec:  *oldProfile,
		},
		{
			name: "LegacyAdheringComponentsOnly uses default Intermediate profile",
			apiServer: configv1.APIServer{
				ObjectMeta: metav1.ObjectMeta{Name: "cluster"},
				Spec: configv1.APIServerSpec{
					TLSSecurityProfile: &configv1.TLSSecurityProfile{
						Type: configv1.TLSProfileOldType,
					},
					TLSAdherence: configv1.TLSAdherencePolicyLegacyAdheringComponentsOnly,
				},
			},
			wantMinTLSVersion:   libgocrypto.TLSVersionOrDie(string(defaultProfile.MinTLSVersion)),
			wantCipherSuites:    true,
			wantAdherencePolicy: configv1.TLSAdherencePolicyLegacyAdheringComponentsOnly,
			wantTLSProfileSpec:  *oldProfile,
		},
		{
			name: "StrictAllComponents honors Modern profile without cipher suites",
			apiServer: configv1.APIServer{
				ObjectMeta: metav1.ObjectMeta{Name: "cluster"},
				Spec: configv1.APIServerSpec{
					TLSSecurityProfile: &configv1.TLSSecurityProfile{
						Type: configv1.TLSProfileModernType,
					},
					TLSAdherence: configv1.TLSAdherencePolicyStrictAllComponents,
				},
			},
			wantMinTLSVersion:   libgocrypto.TLSVersionOrDie(string(modernProfile.MinTLSVersion)),
			wantCipherSuites:    false,
			wantAdherencePolicy: configv1.TLSAdherencePolicyStrictAllComponents,
			wantTLSProfileSpec:  *modernProfile,
		},
		{
			name: "StrictAllComponents honors custom profile",
			apiServer: configv1.APIServer{
				ObjectMeta: metav1.ObjectMeta{Name: "cluster"},
				Spec: configv1.APIServerSpec{
					TLSSecurityProfile: &configv1.TLSSecurityProfile{
						Type: configv1.TLSProfileCustomType,
						Custom: &configv1.CustomTLSProfile{
							TLSProfileSpec: configv1.TLSProfileSpec{
								Ciphers: []string{
									"ECDHE-RSA-AES128-GCM-SHA256",
									"ECDHE-RSA-AES256-GCM-SHA384",
								},
								MinTLSVersion: configv1.VersionTLS12,
							},
						},
					},
					TLSAdherence: configv1.TLSAdherencePolicyStrictAllComponents,
				},
			},
			wantMinTLSVersion: tls.VersionTLS12,
			wantCipherSuites:    true,
			wantAdherencePolicy: configv1.TLSAdherencePolicyStrictAllComponents,
			wantTLSProfileSpec: configv1.TLSProfileSpec{
				Ciphers: []string{
					"ECDHE-RSA-AES128-GCM-SHA256",
					"ECDHE-RSA-AES256-GCM-SHA384",
				},
				MinTLSVersion: configv1.VersionTLS12,
			},
		},
		{
			name: "nil profile defaults to Intermediate when honoring cluster profile",
			apiServer: configv1.APIServer{
				ObjectMeta: metav1.ObjectMeta{Name: "cluster"},
				Spec: configv1.APIServerSpec{
					TLSAdherence: configv1.TLSAdherencePolicyStrictAllComponents,
				},
			},
			wantMinTLSVersion:   libgocrypto.TLSVersionOrDie(string(defaultProfile.MinTLSVersion)),
			wantCipherSuites:    true,
			wantAdherencePolicy: configv1.TLSAdherencePolicyStrictAllComponents,
			wantTLSProfileSpec:  *defaultProfile,
		},
		{
			name: "custom profile without Custom field returns error",
			apiServer: configv1.APIServer{
				ObjectMeta: metav1.ObjectMeta{Name: "cluster"},
				Spec: configv1.APIServerSpec{
					TLSSecurityProfile: &configv1.TLSSecurityProfile{
						Type: configv1.TLSProfileCustomType,
					},
					TLSAdherence: configv1.TLSAdherencePolicyStrictAllComponents,
				},
			},
			wantError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := ResolveTLSConfig(tt.apiServer)
			if tt.wantError {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("ResolveTLSConfig() error = %v", err)
			}

			if result.TLSAdherencePolicy != tt.wantAdherencePolicy {
				t.Fatalf("TLSAdherencePolicy = %q, want %q", result.TLSAdherencePolicy, tt.wantAdherencePolicy)
			}
			if !reflect.DeepEqual(result.TLSProfileSpec, tt.wantTLSProfileSpec) {
				t.Fatalf("TLSProfileSpec = %#v, want %#v", result.TLSProfileSpec, tt.wantTLSProfileSpec)
			}

			tlsCfg := applyTLSConfigResult(t, result)
			if tlsCfg.MinVersion != tt.wantMinTLSVersion {
				t.Fatalf("MinVersion = %d, want %d", tlsCfg.MinVersion, tt.wantMinTLSVersion)
			}
			if tt.wantCipherSuites {
				if len(tlsCfg.CipherSuites) == 0 {
					t.Fatal("expected cipher suites to be configured")
				}
			} else if len(tlsCfg.CipherSuites) != 0 {
				t.Fatalf("expected no cipher suites, got %v", tlsCfg.CipherSuites)
			}
		})
	}
}

func TestFetchAPIServerTLSConfig_clientCreationError(t *testing.T) {
	scheme := runtime.NewScheme()
	if err := configv1.Install(scheme); err != nil {
		t.Fatalf("failed to install configv1 scheme: %v", err)
	}

	_, err := FetchAPIServerTLSConfig(context.Background(), nil, scheme)
	if err == nil {
		t.Fatal("expected error when rest config is nil, got nil")
	}
}

func TestFetchAPIServerTLSConfig_gracefulDefaultsOnFetchFailure(t *testing.T) {
	scheme := runtime.NewScheme()
	if err := configv1.Install(scheme); err != nil {
		t.Fatalf("failed to install configv1 scheme: %v", err)
	}

	result, err := FetchAPIServerTLSConfig(context.Background(), &rest.Config{}, scheme)
	if err != nil {
		t.Fatalf("FetchAPIServerTLSConfig() error = %v", err)
	}

	defaultProfile := configv1.TLSProfiles[libgocrypto.DefaultTLSProfileType]
	if result.TLSAdherencePolicy != configv1.TLSAdherencePolicyNoOpinion {
		t.Fatalf("TLSAdherencePolicy = %q, want empty policy", result.TLSAdherencePolicy)
	}
	if !reflect.DeepEqual(result.TLSProfileSpec, configv1.TLSProfileSpec{}) {
		t.Fatalf("TLSProfileSpec = %#v, want empty profile", result.TLSProfileSpec)
	}

	tlsCfg := applyTLSConfigResult(t, result)
	wantMinVersion := libgocrypto.TLSVersionOrDie(string(defaultProfile.MinTLSVersion))
	if tlsCfg.MinVersion != wantMinVersion {
		t.Fatalf("MinVersion = %d, want %d", tlsCfg.MinVersion, wantMinVersion)
	}
	if len(tlsCfg.CipherSuites) == 0 {
		t.Fatal("expected default Intermediate cipher suites to be configured")
	}
}
