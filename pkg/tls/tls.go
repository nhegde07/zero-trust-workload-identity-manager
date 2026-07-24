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
	"fmt"
	"strings"

	configv1 "github.com/openshift/api/config/v1"
	utiltls "github.com/openshift/controller-runtime-common/pkg/tls"
	libgocrypto "github.com/openshift/library-go/pkg/crypto"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/rest"
	"k8s.io/klog/v2"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// TLSConfigResult holds the resolved TLS configuration along with the cluster-wide TLS profile metadata needed by the SecurityProfileWatcher.
type TLSConfigResult struct {
	// TLSConfig is a function that applies TLS settings to a tls.Config.
	TLSConfig func(*tls.Config)
	// TLSAdherencePolicy is the cluster-wide TLS adherence policy.
	TLSAdherencePolicy configv1.TLSAdherencePolicy
	// TLSProfileSpec is the cluster-wide TLS profile spec.
	TLSProfileSpec configv1.TLSProfileSpec
}

// OperandTLSProfile holds the TLS profile spec for the SPIRE operand.
type OperandTLSProfile struct {
	MinTLSVersion string // Kubernetes TLS version e.g. "VersionTLS10"
	CipherSuites  string // IANA cipher suite names e.g. "TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256,TLS_ECDHE_RSA_WITH_AES_256_GCM_SHA384"
	Curves        string // IANA curve names e.g. "X25519, X25519MLKEM768,secp256r1"
}

// FetchAPIServerTLSConfig fetches operator TLS settings from apiservers/cluster.
func FetchAPIServerTLSConfig(ctx context.Context, restConfig *rest.Config, scheme *runtime.Scheme) (TLSConfigResult, error) {
	k8sClient, err := client.New(restConfig, client.Options{Scheme: scheme})
	if err != nil {
		return TLSConfigResult{}, fmt.Errorf("unable to create Kubernetes client: %w", err)
	}

	initialTLSAdherencePolicy, err := utiltls.FetchAPIServerTLSAdherencePolicy(ctx, k8sClient)
	if err != nil {
		klog.Errorf("error while fetching TLS adherence policy from API server: %v. Continuing with default adherence value: %s", err, string(configv1.TLSAdherencePolicyNoOpinion))
		// Default to empty string if the API server is not available or the field is not set. We will still keep a watch on the API server for the field and trigger a restart if the value changes.
		initialTLSAdherencePolicy = configv1.TLSAdherencePolicyNoOpinion
	}

	initialTLSProfileSpec, err := utiltls.FetchAPIServerTLSProfile(ctx, k8sClient)
	if err != nil {
		klog.Errorf("error while fetching TLS profile from API server: %v. Continuing with empty profile.")
		// Default to an empty profile if the API server is not available or the field is not set. We will still keep a watch on the API server for the field and trigger a restart if the value changes.
		initialTLSProfileSpec = configv1.TLSProfileSpec{}
	}

	return TLSConfigResult{
		TLSConfig:          getTLSConfig(initialTLSAdherencePolicy, initialTLSProfileSpec),
		TLSAdherencePolicy: initialTLSAdherencePolicy,
		TLSProfileSpec:     initialTLSProfileSpec,
	}, nil
}

func getTLSConfig(tlsAdherencePolicy configv1.TLSAdherencePolicy, tlsProfileSpec configv1.TLSProfileSpec) func(*tls.Config) {
	var tlsConfig func(*tls.Config)

	// If the cluster-wide TLS adherence policy is set to honor the cluster-wide TLS profile,
	// use the cluster-wide TLS profile-based configuration.
	if libgocrypto.ShouldHonorClusterTLSProfile(tlsAdherencePolicy) {
		profileTLSConfig, unsupportedCiphers := utiltls.NewTLSConfigFromProfile(tlsProfileSpec)
		if len(unsupportedCiphers) > 0 {
			klog.Infof("TLS configuration contains unsupported ciphers that will be ignored: %v", unsupportedCiphers)
		}

		// Set the TLS configuration to the cluster-wide TLS profile-based configuration.
		tlsConfig = profileTLSConfig
	} else {
		//Do nothing. Let The TLS Endpoints behave as they are.
		tlsConfig = nil
	}

	return tlsConfig
}

// GetOperandTLSProfile resolves the config.openshift.io/v1/apiserver resource into a OperandTLSProfile.
func GetOperandTLSProfile(apiServer configv1.APIServer) (OperandTLSProfile, error) {
	tlsProfileSpec, err := utiltls.GetTLSProfileSpec(apiServer.Spec.TLSSecurityProfile)
	tlsCfg := getTLSConfig(apiServer.Spec.TLSAdherence, tlsProfileSpec)

	if err != nil {
		return getOperandTLSProfileFromTLSConfig(tlsCfg, tlsProfileSpec), fmt.Errorf("error while fetching TLS profile: %w", err)
	}

	return getOperandTLSProfileFromTLSConfig(tlsCfg, tlsProfileSpec), nil
}

func getOperandTLSProfileFromTLSConfig(tlsCfg func(*tls.Config), tlsProfileSpec configv1.TLSProfileSpec) OperandTLSProfile {
	if tlsCfg == nil {
		return OperandTLSProfile{}
	}

	profile := OperandTLSProfile{
		MinTLSVersion: string(tlsProfileSpec.MinTLSVersion),
		CipherSuites:  joinIANACiphers(tlsProfileSpec.Ciphers),
	}

	return profile
}

func joinIANACiphers(openSSLNames []string) string {
	iana := libgocrypto.OpenSSLToIANACipherSuites(openSSLNames)
	return strings.Join(iana, ",")
}

// InsertOperandTLSProfileToConfigMap returns the experimental.tlsProfile block for SPIRE operand configs.
func InsertOperandTLSProfileToConfigMap(config map[string]interface{}, profile OperandTLSProfile) {
	config["experimental"] = map[string]interface{}{
		"tlsProfile": map[string]interface{}{
			"minTLSVersion": profile.MinTLSVersion,
			"cipherSuites":  profile.CipherSuites,
			"curves":        profile.Curves,
		},
	}
}

/*
{
  "experimental": {
    "tlsProfile": {
      "minTLSVersion": "VersionTLS12",
      "cipherSuites": [
        "TLS_AES_128_GCM_SHA256",
        "TLS_AES_256_GCM_SHA384",
        "TLS_CHACHA20_POLY1305_SHA256",
        "TLS_ECDHE_ECDSA_WITH_AES_128_GCM_SHA256",
        "TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256",
        "TLS_ECDHE_ECDSA_WITH_AES_256_GCM_SHA384",
        "TLS_ECDHE_RSA_WITH_AES_256_GCM_SHA384",
        "TLS_ECDHE_ECDSA_WITH_CHACHA20_POLY1305_SHA256",
        "TLS_ECDHE_RSA_WITH_CHACHA20_POLY1305_SHA256"
      ],
      "curves": [
        "X25519MLKEM768",
        "X25519",
        "secp256r1",
        "secp384r1"
      ]
    }
  }
}
*/
