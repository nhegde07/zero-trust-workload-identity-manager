/*
Copyright 2025.

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
	"errors"
	"testing"

	configv1 "github.com/openshift/api/config/v1"
	"github.com/openshift/zero-trust-workload-identity-manager/api/v1alpha1"
	"github.com/openshift/zero-trust-workload-identity-manager/pkg/client/fakes"
	kerrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func TestResolveEffectiveTLSConfig(t *testing.T) {
	t.Parallel()

	intermediate := intermediateProfileSpec()
	modern := *configv1.TLSProfiles[configv1.TLSProfileModernType]
	old := *configv1.TLSProfiles[configv1.TLSProfileOldType]

	tests := []struct {
		name              string
		apiServer         *configv1.APIServer
		getErr            error
		wantSource        InjectionSource
		wantStrict        bool
		wantRequirePQKEM  bool
		wantInjectCentral bool
		wantMinTLSVersion configv1.TLSProtocolVersion
		wantCipherCount   int
		requirePQKEM      *bool
	}{
		{
			name: "strict modern profile from APIServer",
			apiServer: &configv1.APIServer{
				ObjectMeta: metav1.ObjectMeta{Name: "cluster"},
				Spec: configv1.APIServerSpec{
					TLSAdherence: configv1.TLSAdherencePolicyStrictAllComponents,
					TLSSecurityProfile: &configv1.TLSSecurityProfile{
						Type: configv1.TLSProfileModernType,
					},
				},
			},
			wantSource:        InjectionSourceClusterProfile,
			wantStrict:        true,
			wantInjectCentral: true,
			wantMinTLSVersion: modern.MinTLSVersion,
			wantCipherCount:   len(modern.Ciphers),
		},
		{
			name: "strict old profile from APIServer",
			apiServer: &configv1.APIServer{
				ObjectMeta: metav1.ObjectMeta{Name: "cluster"},
				Spec: configv1.APIServerSpec{
					TLSAdherence: configv1.TLSAdherencePolicyStrictAllComponents,
					TLSSecurityProfile: &configv1.TLSSecurityProfile{
						Type: configv1.TLSProfileOldType,
					},
				},
			},
			wantSource:        InjectionSourceClusterProfile,
			wantStrict:        true,
			wantInjectCentral: true,
			wantMinTLSVersion: old.MinTLSVersion,
			wantCipherCount:   len(old.Ciphers),
		},
		{
			name: "strict intermediate profile from APIServer",
			apiServer: &configv1.APIServer{
				ObjectMeta: metav1.ObjectMeta{Name: "cluster"},
				Spec: configv1.APIServerSpec{
					TLSAdherence: configv1.TLSAdherencePolicyStrictAllComponents,
					TLSSecurityProfile: &configv1.TLSSecurityProfile{
						Type: configv1.TLSProfileIntermediateType,
					},
				},
			},
			wantSource:        InjectionSourceClusterProfile,
			wantStrict:        true,
			wantInjectCentral: true,
			wantMinTLSVersion: intermediate.MinTLSVersion,
			wantCipherCount:   len(intermediate.Ciphers),
		},
		{
			name: "strict custom profile",
			apiServer: &configv1.APIServer{
				ObjectMeta: metav1.ObjectMeta{Name: "cluster"},
				Spec: configv1.APIServerSpec{
					TLSAdherence: configv1.TLSAdherencePolicyStrictAllComponents,
					TLSSecurityProfile: &configv1.TLSSecurityProfile{
						Type: configv1.TLSProfileCustomType,
						Custom: &configv1.CustomTLSProfile{
							TLSProfileSpec: configv1.TLSProfileSpec{
								MinTLSVersion: configv1.VersionTLS12,
								Ciphers:       []string{"TLS_AES_128_GCM_SHA256"},
							},
						},
					},
				},
			},
			wantSource:        InjectionSourceClusterProfile,
			wantStrict:        true,
			wantInjectCentral: true,
			wantMinTLSVersion: configv1.VersionTLS12,
			wantCipherCount:   1,
		},
		{
			name: "non-strict legacy adherence uses intermediate baseline",
			apiServer: &configv1.APIServer{
				ObjectMeta: metav1.ObjectMeta{Name: "cluster"},
				Spec: configv1.APIServerSpec{
					TLSAdherence: configv1.TLSAdherencePolicyLegacyAdheringComponentsOnly,
					TLSSecurityProfile: &configv1.TLSSecurityProfile{
						Type: configv1.TLSProfileModernType,
					},
				},
			},
			wantSource:        InjectionSourceIntermediateDefault,
			wantStrict:        false,
			wantInjectCentral: false,
			wantMinTLSVersion: intermediate.MinTLSVersion,
			wantCipherCount:   len(intermediate.Ciphers),
		},
		{
			name: "no-opinion adherence uses intermediate baseline",
			apiServer: &configv1.APIServer{
				ObjectMeta: metav1.ObjectMeta{Name: "cluster"},
				Spec: configv1.APIServerSpec{
					TLSSecurityProfile: &configv1.TLSSecurityProfile{
						Type: configv1.TLSProfileModernType,
					},
				},
			},
			wantSource:        InjectionSourceIntermediateDefault,
			wantStrict:        false,
			wantInjectCentral: false,
			wantMinTLSVersion: intermediate.MinTLSVersion,
			wantCipherCount:   len(intermediate.Ciphers),
		},
		{
			name:              "strict APIServer get failure falls back to intermediate",
			getErr:            kerrors.NewNotFound(schema.GroupResource{Group: "config.openshift.io", Resource: "apiservers"}, "cluster"),
			wantSource:        InjectionSourceIntermediateDefault,
			wantStrict:        false,
			wantInjectCentral: false,
			wantMinTLSVersion: intermediate.MinTLSVersion,
			wantCipherCount:   len(intermediate.Ciphers),
		},
		{
			name: "strict custom profile nil falls back to intermediate",
			apiServer: &configv1.APIServer{
				ObjectMeta: metav1.ObjectMeta{Name: "cluster"},
				Spec: configv1.APIServerSpec{
					TLSAdherence: configv1.TLSAdherencePolicyStrictAllComponents,
					TLSSecurityProfile: &configv1.TLSSecurityProfile{
						Type: configv1.TLSProfileCustomType,
					},
				},
			},
			wantSource:        InjectionSourceIntermediateDefault,
			wantStrict:        true,
			wantInjectCentral: true,
			wantMinTLSVersion: intermediate.MinTLSVersion,
			wantCipherCount:   len(intermediate.Ciphers),
		},
		{
			name: "requirePQKEM true overrides strict modern profile",
			apiServer: &configv1.APIServer{
				ObjectMeta: metav1.ObjectMeta{Name: "cluster"},
				Spec: configv1.APIServerSpec{
					TLSAdherence: configv1.TLSAdherencePolicyStrictAllComponents,
					TLSSecurityProfile: &configv1.TLSSecurityProfile{
						Type: configv1.TLSProfileModernType,
					},
				},
			},
			requirePQKEM:      boolPtr(true),
			wantSource:        InjectionSourcePQCOverride,
			wantStrict:        true,
			wantRequirePQKEM:  true,
			wantInjectCentral: false,
		},
		{
			name: "requirePQKEM false preserves strict profile path",
			apiServer: &configv1.APIServer{
				ObjectMeta: metav1.ObjectMeta{Name: "cluster"},
				Spec: configv1.APIServerSpec{
					TLSAdherence: configv1.TLSAdherencePolicyStrictAllComponents,
					TLSSecurityProfile: &configv1.TLSSecurityProfile{
						Type: configv1.TLSProfileModernType,
					},
				},
			},
			requirePQKEM:      boolPtr(false),
			wantSource:        InjectionSourceClusterProfile,
			wantStrict:        true,
			wantRequirePQKEM:  false,
			wantInjectCentral: true,
			wantMinTLSVersion: modern.MinTLSVersion,
			wantCipherCount:   len(modern.Ciphers),
		},
		{
			name: "requirePQKEM true with non-strict adherence",
			apiServer: &configv1.APIServer{
				ObjectMeta: metav1.ObjectMeta{Name: "cluster"},
				Spec: configv1.APIServerSpec{
					TLSAdherence: configv1.TLSAdherencePolicyNoOpinion,
				},
			},
			requirePQKEM:      boolPtr(true),
			wantSource:        InjectionSourcePQCOverride,
			wantStrict:        false,
			wantRequirePQKEM:  true,
			wantInjectCentral: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			fakeClient := newFakeClientWithAPIServer(tt.apiServer, tt.getErr)
			ztwim := &v1alpha1.ZeroTrustWorkloadIdentityManager{
				ObjectMeta: metav1.ObjectMeta{Name: "cluster"},
				Spec: v1alpha1.ZeroTrustWorkloadIdentityManagerSpec{
					RequirePQKEM: tt.requirePQKEM,
				},
			}

			cfg, err := ResolveEffectiveTLSConfig(context.Background(), fakeClient, ztwim)
			if err != nil {
				t.Fatalf("ResolveEffectiveTLSConfig() error = %v", err)
			}

			if cfg.Source != tt.wantSource {
				t.Errorf("Source = %q, want %q", cfg.Source, tt.wantSource)
			}
			if cfg.StrictAdherence != tt.wantStrict {
				t.Errorf("StrictAdherence = %v, want %v", cfg.StrictAdherence, tt.wantStrict)
			}
			if cfg.RequirePQKEM != tt.wantRequirePQKEM {
				t.Errorf("RequirePQKEM = %v, want %v", cfg.RequirePQKEM, tt.wantRequirePQKEM)
			}
			if cfg.ProfileSpec.MinTLSVersion != tt.wantMinTLSVersion {
				t.Errorf("MinTLSVersion = %q, want %q", cfg.ProfileSpec.MinTLSVersion, tt.wantMinTLSVersion)
			}
			if len(cfg.ProfileSpec.Ciphers) != tt.wantCipherCount {
				t.Errorf("cipher count = %d, want %d", len(cfg.ProfileSpec.Ciphers), tt.wantCipherCount)
			}
			if got := ShouldInjectCentralProfile(cfg); got != tt.wantInjectCentral {
				t.Errorf("ShouldInjectCentralProfile() = %v, want %v", got, tt.wantInjectCentral)
			}

			operandCfg := ToOperandTLSConfig(cfg)
			if operandCfg.RequirePQKEM != tt.wantRequirePQKEM {
				t.Errorf("Operand RequirePQKEM = %v, want %v", operandCfg.RequirePQKEM, tt.wantRequirePQKEM)
			}
			if operandCfg.Inject != tt.wantInjectCentral {
				t.Errorf("Operand Inject = %v, want %v", operandCfg.Inject, tt.wantInjectCentral)
			}
		})
	}
}

func TestShouldInjectPQKEM(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		require *bool
		want    bool
	}{
		{name: "nil", require: nil, want: false},
		{name: "false", require: boolPtr(false), want: false},
		{name: "true", require: boolPtr(true), want: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ztwim := &v1alpha1.ZeroTrustWorkloadIdentityManager{
				Spec: v1alpha1.ZeroTrustWorkloadIdentityManagerSpec{RequirePQKEM: tt.require},
			}
			if got := ShouldInjectPQKEM(ztwim); got != tt.want {
				t.Errorf("ShouldInjectPQKEM() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestResolveTLSProfile(t *testing.T) {
	t.Parallel()

	modern := *configv1.TLSProfiles[configv1.TLSProfileModernType]
	fakeClient := newFakeClientWithAPIServer(&configv1.APIServer{
		ObjectMeta: metav1.ObjectMeta{Name: "cluster"},
		Spec: configv1.APIServerSpec{
			TLSSecurityProfile: &configv1.TLSSecurityProfile{
				Type: configv1.TLSProfileModernType,
			},
		},
	}, nil)

	profile, err := ResolveTLSProfile(context.Background(), fakeClient)
	if err != nil {
		t.Fatalf("ResolveTLSProfile() error = %v", err)
	}
	if profile.MinTLSVersion != modern.MinTLSVersion {
		t.Errorf("MinTLSVersion = %q, want %q", profile.MinTLSVersion, modern.MinTLSVersion)
	}
}

func TestResolveTLSProfile_APIServerGetFailure(t *testing.T) {
	t.Parallel()

	fakeClient := &fakes.FakeCustomCtrlClient{}
	fakeClient.GetStub = func(_ context.Context, _ client.ObjectKey, _ client.Object) error {
		return errors.New("connection refused")
	}

	_, err := ResolveTLSProfile(context.Background(), fakeClient)
	if err == nil {
		t.Fatal("ResolveTLSProfile() expected error, got nil")
	}
}

func newFakeClientWithAPIServer(apiServer *configv1.APIServer, getErr error) *fakes.FakeCustomCtrlClient {
	fakeClient := &fakes.FakeCustomCtrlClient{}
	fakeClient.GetStub = func(_ context.Context, key client.ObjectKey, obj client.Object) error {
		if getErr != nil {
			return getErr
		}

		apiserver, ok := obj.(*configv1.APIServer)
		if !ok {
			return errors.New("unexpected object type")
		}
		if key.Name != "cluster" {
			return kerrors.NewNotFound(schema.GroupResource{Group: "config.openshift.io", Resource: "apiservers"}, key.Name)
		}

		*apiserver = *apiServer.DeepCopy()
		return nil
	}

	return fakeClient
}

func TestApplyOperandTLSConfig(t *testing.T) {
	t.Parallel()

	intermediate := intermediateProfileSpec()
	modern := *configv1.TLSProfiles[configv1.TLSProfileModernType]

	tests := []struct {
		name            string
		operandCfg      OperandTLSConfig
		wantMinTLS      string
		wantCiphers     bool
		wantCipherCount int
	}{
		{
			name: "non-strict skip",
			operandCfg: OperandTLSConfig{
				Inject: false,
			},
		},
		{
			name: "strict TLS 1.2 injects ciphers",
			operandCfg: OperandTLSConfig{
				Inject:       true,
				MinVersion:   intermediate.MinTLSVersion,
				CipherSuites: intermediate.Ciphers,
			},
			wantMinTLS:      "1.2",
			wantCiphers:     true,
			wantCipherCount: len(intermediate.Ciphers),
		},
		{
			name: "strict TLS 1.3 omits ciphers",
			operandCfg: OperandTLSConfig{
				Inject:       true,
				MinVersion:   modern.MinTLSVersion,
				CipherSuites: modern.Ciphers,
			},
			wantMinTLS:  "1.3",
			wantCiphers: false,
		},
		{
			name: "strict inject with empty cipher list",
			operandCfg: OperandTLSConfig{
				Inject:       true,
				MinVersion:   intermediate.MinTLSVersion,
				CipherSuites: nil,
			},
			wantMinTLS:  "1.2",
			wantCiphers: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config := map[string]interface{}{"server": map[string]interface{}{}}
			ApplyOperandTLSConfig(config, tt.operandCfg)

			if tt.wantMinTLS == "" {
				if _, ok := config[SPIREConfigKeyMinTLSVersion]; ok {
					t.Fatalf("expected no %q key", SPIREConfigKeyMinTLSVersion)
				}
				return
			}

			gotMin, ok := config[SPIREConfigKeyMinTLSVersion].(string)
			if !ok || gotMin != tt.wantMinTLS {
				t.Fatalf("minTLSVersion = %v, want %q", config[SPIREConfigKeyMinTLSVersion], tt.wantMinTLS)
			}

			ciphersRaw, hasCiphers := config[SPIREConfigKeyCipherSuites]
			if tt.wantCiphers != hasCiphers {
				t.Fatalf("cipherSuites present = %v, want %v", hasCiphers, tt.wantCiphers)
			}
			if tt.wantCiphers {
				ciphers, ok := ciphersRaw.([]interface{})
				if !ok || len(ciphers) != tt.wantCipherCount {
					t.Fatalf("cipherSuites len = %d, want %d", len(ciphers), tt.wantCipherCount)
				}
			}
		})
	}
}

func TestApplyOperandTLSConfigToPrometheus(t *testing.T) {
	t.Parallel()

	intermediate := intermediateProfileSpec()
	telemetry := map[string]interface{}{
		"Prometheus": map[string]interface{}{
			"host": "0.0.0.0",
			"port": "9402",
		},
	}

	ApplyOperandTLSConfigToPrometheus(telemetry, OperandTLSConfig{})
	prom := telemetry["Prometheus"].(map[string]interface{})
	if _, ok := prom[SPIREConfigKeyMinTLSVersion]; ok {
		t.Fatal("expected no TLS injection when Inject is false")
	}

	ApplyOperandTLSConfigToPrometheus(telemetry, OperandTLSConfig{
		Inject:       true,
		MinVersion:   intermediate.MinTLSVersion,
		CipherSuites: intermediate.Ciphers,
	})
	if prom[SPIREConfigKeyMinTLSVersion] != "1.2" {
		t.Fatalf("Prometheus minTLSVersion = %v, want 1.2", prom[SPIREConfigKeyMinTLSVersion])
	}
	if _, ok := prom[SPIREConfigKeyCipherSuites]; !ok {
		t.Fatal("expected cipherSuites in Prometheus block")
	}
}

func TestSPIREMinTLSVersion(t *testing.T) {
	t.Parallel()

	tests := []struct {
		in   configv1.TLSProtocolVersion
		want string
	}{
		{configv1.VersionTLS10, "1.0"},
		{configv1.VersionTLS11, "1.1"},
		{configv1.VersionTLS12, "1.2"},
		{configv1.VersionTLS13, "1.3"},
		{configv1.TLSProtocolVersion("unknown"), ""},
	}
	for _, tt := range tests {
		if got := SPIREMinTLSVersion(tt.in); got != tt.want {
			t.Errorf("SPIREMinTLSVersion(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func boolPtr(v bool) *bool {
	return &v
}

func TestApplyOperandPQKEMConfig(t *testing.T) {
	t.Parallel()

	config := map[string]interface{}{}
	ApplyOperandPQKEMConfig(config, OperandTLSConfig{RequirePQKEM: true})
	experimental, ok := config[SPIREConfigKeyExperimental].(map[string]interface{})
	if !ok || experimental[SPIREConfigKeyRequirePQKEM] != true {
		t.Fatalf("expected experimental require_pq_kem, got %v", config[SPIREConfigKeyExperimental])
	}

	config2 := map[string]interface{}{}
	ApplyOperandPQKEMConfig(config2, OperandTLSConfig{})
	if _, ok := config2[SPIREConfigKeyExperimental]; ok {
		t.Fatal("expected no experimental block when RequirePQKEM is false")
	}
}

func TestApplyOperandTLSSettings(t *testing.T) {
	t.Parallel()

	intermediate := intermediateProfileSpec()
	pqcConfig := map[string]interface{}{}
	ApplyOperandTLSSettings(pqcConfig, OperandTLSConfig{RequirePQKEM: true})
	if _, ok := pqcConfig[SPIREConfigKeyMinTLSVersion]; ok {
		t.Fatal("expected no central TLS keys when PQC active")
	}
	if _, ok := pqcConfig[SPIREConfigKeyExperimental]; !ok {
		t.Fatal("expected experimental block when PQC active")
	}

	centralConfig := map[string]interface{}{}
	ApplyOperandTLSSettings(centralConfig, OperandTLSConfig{
		Inject:       true,
		MinVersion:   intermediate.MinTLSVersion,
		CipherSuites: intermediate.Ciphers,
	})
	if centralConfig[SPIREConfigKeyMinTLSVersion] != "1.2" {
		t.Fatalf("expected central TLS injection, got %v", centralConfig)
	}
}

func TestRecordPQKEStrictAdherenceWarning(t *testing.T) {
	t.Parallel()

	recorder := record.NewFakeRecorder(10)
	ztwim := &v1alpha1.ZeroTrustWorkloadIdentityManager{ObjectMeta: metav1.ObjectMeta{Name: "cluster"}}

	RecordPQKEStrictAdherenceWarning(recorder, ztwim, EffectiveTLSConfig{
		RequirePQKEM:    true,
		StrictAdherence: true,
	})
	select {
	case event := <-recorder.Events:
		if event != "Warning "+EventReasonPQKESOverridesCentralTLSProfile+" "+EventMessagePQKESOverridesCentralTLSProfile {
			t.Fatalf("unexpected event: %q", event)
		}
	default:
		t.Fatal("expected warning event")
	}

	RecordPQKEStrictAdherenceWarning(recorder, ztwim, EffectiveTLSConfig{
		RequirePQKEM:    false,
		StrictAdherence: true,
	})
	select {
	case event := <-recorder.Events:
		t.Fatalf("unexpected event when PQC disabled: %q", event)
	default:
	}
}
