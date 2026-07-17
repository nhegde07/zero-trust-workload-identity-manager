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
	"fmt"

	configv1 "github.com/openshift/api/config/v1"
	commontls "github.com/openshift/controller-runtime-common/pkg/tls"
	"github.com/openshift/zero-trust-workload-identity-manager/api/v1alpha1"
	"github.com/openshift/zero-trust-workload-identity-manager/pkg/client"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/record"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"
)

// InjectionSource identifies how the effective TLS configuration was resolved.
type InjectionSource string

const (
	// InjectionSourceClusterProfile indicates settings were resolved from the cluster APIServer TLS profile.
	InjectionSourceClusterProfile InjectionSource = "ClusterProfile"
	// InjectionSourceIntermediateDefault indicates the Intermediate baseline profile is in effect.
	InjectionSourceIntermediateDefault InjectionSource = "IntermediateDefault"
	// InjectionSourcePQCOverride indicates hybrid PQC settings take precedence over the central profile.
	InjectionSourcePQCOverride InjectionSource = "PQCOverride"
)

// EffectiveTLSConfig holds the resolved TLS settings and precedence metadata for operator and operand wiring.
type EffectiveTLSConfig struct {
	ProfileSpec     configv1.TLSProfileSpec
	StrictAdherence bool
	RequirePQKEM    bool
	Source          InjectionSource
}

// OperandTLSConfig is the call-chain struct consumed by operand configmap generators.
type OperandTLSConfig struct {
	Inject       bool
	MinVersion   configv1.TLSProtocolVersion
	CipherSuites []string
	RequirePQKEM bool
}

const (
	// SPIREConfigKeyMinTLSVersion is the SPIRE JSON/HCL key for minimum TLS version.
	SPIREConfigKeyMinTLSVersion = "minTLSVersion"
	// SPIREConfigKeyCipherSuites is the SPIRE JSON/HCL key for cipher suite overrides.
	SPIREConfigKeyCipherSuites = "cipherSuites"
	// SPIREConfigKeyExperimental is the SPIRE JSON/HCL key for experimental settings.
	SPIREConfigKeyExperimental = "experimental"
	// SPIREConfigKeyRequirePQKEM is the SPIRE experimental key for hybrid PQC enforcement.
	SPIREConfigKeyRequirePQKEM = "require_pq_kem"
)

const (
	// EventReasonPQKESOverridesCentralTLSProfile is emitted when requirePQKEM and strict adherence coexist.
	EventReasonPQKESOverridesCentralTLSProfile = "PQKESOverridesCentralTLSProfile"
	// EventMessagePQKESOverridesCentralTLSProfile explains PQC precedence over the central TLS profile.
	EventMessagePQKESOverridesCentralTLSProfile = "spec.requirePQKEM is enabled while tlsAdherencePolicy is StrictAllComponents; application-level PQC policy overrides central TLS profile injection into SPIRE operand configs"
)

// SPIREMinTLSVersion maps an OpenShift TLS protocol version to a SPIRE-compatible string.
func SPIREMinTLSVersion(version configv1.TLSProtocolVersion) string {
	switch version {
	case configv1.VersionTLS10:
		return "1.0"
	case configv1.VersionTLS11:
		return "1.1"
	case configv1.VersionTLS12:
		return "1.2"
	case configv1.VersionTLS13:
		return "1.3"
	default:
		return ""
	}
}

// ShouldInjectCipherSuites reports whether cipher suites should be injected for the min TLS version.
// TLS 1.3 profiles omit cipher suites per Go/SPIRE negotiation behavior.
func ShouldInjectCipherSuites(minVersion configv1.TLSProtocolVersion) bool {
	return minVersion != configv1.VersionTLS13
}

func injectTLSFields(target map[string]interface{}, operandCfg OperandTLSConfig) {
	if minTLS := SPIREMinTLSVersion(operandCfg.MinVersion); minTLS != "" {
		target[SPIREConfigKeyMinTLSVersion] = minTLS
	}
	if ShouldInjectCipherSuites(operandCfg.MinVersion) && len(operandCfg.CipherSuites) > 0 {
		ciphers := make([]interface{}, len(operandCfg.CipherSuites))
		for i, cipher := range operandCfg.CipherSuites {
			ciphers[i] = cipher
		}
		target[SPIREConfigKeyCipherSuites] = ciphers
	}
}

// ApplyOperandTLSConfig injects central TLS profile settings into a SPIRE config map when enabled.
func ApplyOperandTLSConfig(config map[string]interface{}, operandCfg OperandTLSConfig) {
	if config == nil || !operandCfg.Inject {
		return
	}
	injectTLSFields(config, operandCfg)
}

// ApplyOperandTLSConfigToPrometheus injects TLS settings into telemetry.Prometheus blocks.
func ApplyOperandTLSConfigToPrometheus(telemetry map[string]interface{}, operandCfg OperandTLSConfig) {
	if telemetry == nil || !operandCfg.Inject {
		return
	}
	promRaw, ok := telemetry["Prometheus"]
	if !ok {
		return
	}
	prom, ok := promRaw.(map[string]interface{})
	if !ok {
		return
	}
	injectTLSFields(prom, operandCfg)
}

// ApplyOperandPQKEMConfig injects SPIRE experimental.require_pq_kem when hybrid PQC is enabled.
func ApplyOperandPQKEMConfig(config map[string]interface{}, operandCfg OperandTLSConfig) {
	if config == nil || !operandCfg.RequirePQKEM {
		return
	}
	config[SPIREConfigKeyExperimental] = map[string]interface{}{
		SPIREConfigKeyRequirePQKEM: true,
	}
}

// ApplyOperandTLSSettings applies PQC or central-profile TLS settings (mutually exclusive).
func ApplyOperandTLSSettings(config map[string]interface{}, operandCfg OperandTLSConfig) {
	if operandCfg.RequirePQKEM {
		ApplyOperandPQKEMConfig(config, operandCfg)
		return
	}
	ApplyOperandTLSConfig(config, operandCfg)
}

// ApplyOperandTLSSettingsToPrometheus applies TLS or PQC settings to telemetry.Prometheus blocks.
func ApplyOperandTLSSettingsToPrometheus(telemetry map[string]interface{}, operandCfg OperandTLSConfig) {
	if operandCfg.RequirePQKEM {
		return
	}
	ApplyOperandTLSConfigToPrometheus(telemetry, operandCfg)
}

// RecordPQKEStrictAdherenceWarning emits a warning when PQC override coexists with strict TLS adherence.
func RecordPQKEStrictAdherenceWarning(recorder record.EventRecorder, obj runtime.Object, cfg EffectiveTLSConfig) {
	if recorder == nil || obj == nil {
		return
	}
	if cfg.RequirePQKEM && cfg.StrictAdherence {
		recorder.Event(obj, corev1.EventTypeWarning, EventReasonPQKESOverridesCentralTLSProfile, EventMessagePQKESOverridesCentralTLSProfile)
	}
}

// intermediateProfileSpec returns the cluster Intermediate TLS baseline.
func intermediateProfileSpec() configv1.TLSProfileSpec {
	return *configv1.TLSProfiles[configv1.TLSProfileIntermediateType]
}

// isStrictAdherence reports whether the cluster policy requires all components to honor the TLS profile.
func isStrictAdherence(policy configv1.TLSAdherencePolicy) bool {
	return policy == configv1.TLSAdherencePolicyStrictAllComponents
}

// isRequirePQKEMEnabled reports whether hybrid PQC is enabled on the ZTWIM CR.
func isRequirePQKEMEnabled(ztwim *v1alpha1.ZeroTrustWorkloadIdentityManager) bool {
	if ztwim == nil || ztwim.Spec.RequirePQKEM == nil {
		return false
	}
	return *ztwim.Spec.RequirePQKEM
}

// ShouldInjectPQKEM reports whether operand configs should receive hybrid PQC settings.
func ShouldInjectPQKEM(ztwim *v1alpha1.ZeroTrustWorkloadIdentityManager) bool {
	return isRequirePQKEMEnabled(ztwim)
}

// ShouldInjectCentralProfile gates central TLS profile injection into operand configs.
// Injection is enabled only under strict adherence when PQC override is not active.
func ShouldInjectCentralProfile(cfg EffectiveTLSConfig) bool {
	return !cfg.RequirePQKEM && cfg.StrictAdherence
}

func fetchAPIServer(ctx context.Context, k8sClient client.CustomCtrlClient) (*configv1.APIServer, error) {
	apiServer := &configv1.APIServer{}
	key := ctrlclient.ObjectKey{Name: commontls.APIServerName}
	if err := k8sClient.Get(ctx, key, apiServer); err != nil {
		return nil, fmt.Errorf("failed to get APIServer %q: %w", key.String(), err)
	}

	return apiServer, nil
}

func fetchAPIServerTLSAdherencePolicy(ctx context.Context, k8sClient client.CustomCtrlClient) (configv1.TLSAdherencePolicy, error) {
	apiServer, err := fetchAPIServer(ctx, k8sClient)
	if err != nil {
		return configv1.TLSAdherencePolicyNoOpinion, err
	}

	return apiServer.Spec.TLSAdherence, nil
}

func fetchAPIServerTLSProfile(ctx context.Context, k8sClient client.CustomCtrlClient) (configv1.TLSProfileSpec, error) {
	apiServer, err := fetchAPIServer(ctx, k8sClient)
	if err != nil {
		return configv1.TLSProfileSpec{}, err
	}

	profile, err := commontls.GetTLSProfileSpec(apiServer.Spec.TLSSecurityProfile)
	if err != nil {
		return configv1.TLSProfileSpec{}, fmt.Errorf("failed to get TLS profile from APIServer %q: %w", commontls.APIServerName, err)
	}

	return profile, nil
}

// ResolveTLSProfile fetches the TLS profile spec configured on the cluster APIServer.
func ResolveTLSProfile(ctx context.Context, k8sClient client.CustomCtrlClient) (configv1.TLSProfileSpec, error) {
	return fetchAPIServerTLSProfile(ctx, k8sClient)
}

// ResolveEffectiveTLSConfig resolves TLS settings using FR-014 precedence:
// PQC override > strict cluster profile > non-strict Intermediate defaults > strict Intermediate fallback on fetch/parse failure.
func ResolveEffectiveTLSConfig(
	ctx context.Context,
	k8sClient client.CustomCtrlClient,
	ztwim *v1alpha1.ZeroTrustWorkloadIdentityManager,
) (EffectiveTLSConfig, error) {
	if isRequirePQKEMEnabled(ztwim) {
		adherence, _ := fetchAPIServerTLSAdherencePolicy(ctx, k8sClient)
		return EffectiveTLSConfig{
			StrictAdherence: isStrictAdherence(adherence),
			RequirePQKEM:    true,
			Source:          InjectionSourcePQCOverride,
		}, nil
	}

	adherence, adherenceErr := fetchAPIServerTLSAdherencePolicy(ctx, k8sClient)
	if adherenceErr != nil || !isStrictAdherence(adherence) {
		return EffectiveTLSConfig{
			ProfileSpec:     intermediateProfileSpec(),
			StrictAdherence: false,
			RequirePQKEM:    false,
			Source:          InjectionSourceIntermediateDefault,
		}, nil
	}

	profile, err := fetchAPIServerTLSProfile(ctx, k8sClient)
	if err != nil {
		return EffectiveTLSConfig{
			ProfileSpec:     intermediateProfileSpec(),
			StrictAdherence: true,
			RequirePQKEM:    false,
			Source:          InjectionSourceIntermediateDefault,
		}, nil
	}

	return EffectiveTLSConfig{
		ProfileSpec:     profile,
		StrictAdherence: true,
		RequirePQKEM:    false,
		Source:          InjectionSourceClusterProfile,
	}, nil
}

// ToOperandTLSConfig converts a resolved effective config into operand generator inputs.
func ToOperandTLSConfig(cfg EffectiveTLSConfig) OperandTLSConfig {
	if cfg.RequirePQKEM {
		return OperandTLSConfig{
			RequirePQKEM: true,
		}
	}

	if !ShouldInjectCentralProfile(cfg) {
		return OperandTLSConfig{}
	}

	return OperandTLSConfig{
		Inject:       true,
		MinVersion:   cfg.ProfileSpec.MinTLSVersion,
		CipherSuites: append([]string(nil), cfg.ProfileSpec.Ciphers...),
	}
}
