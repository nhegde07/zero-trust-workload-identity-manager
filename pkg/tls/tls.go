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

// FetchAPIServerTLSConfig fetches the TLS configuration from the cluster's apiservers.config.openshift.io/cluster resource and applies it based on the adherence policy.
func FetchAPIServerTLSConfig(ctx context.Context, restConfig *rest.Config, scheme *runtime.Scheme) (TLSConfigResult, error) {
	k8sClient, err := client.New(restConfig, client.Options{Scheme: scheme})
	if err != nil {
		return TLSConfigResult{}, fmt.Errorf("unable to create Kubernetes client: %w", err)
	}

	tlsAdherencePolicy, err := utiltls.FetchAPIServerTLSAdherencePolicy(ctx, k8sClient)
	if err != nil {
		klog.Errorf("unable to get TLS adherence policy from API server: %v", err)
		// Default to empty string if the API server is not available or the field is not set.
		// We will still keep a watch on the API server for the field and trigger a restart if the value changes.
		tlsAdherencePolicy = ""
	}

	tlsProfileSpec, err := utiltls.FetchAPIServerTLSProfile(ctx, k8sClient)
	if err != nil {
		klog.Errorf("unable to get TLS profile from API server: %v", err)
		// Default to an empty profile if the API server is not available or the field is not set.
		// We will still keep a watch on the API server for the field and trigger a restart if the value changes.
		tlsProfileSpec = configv1.TLSProfileSpec{}
	}

	return getTLSConfig(tlsAdherencePolicy, tlsProfileSpec)

}

// ResolveTLSConfig resolves the config.openshift.io/v1/apiservers resource into a TLSConfigResult.
func ResolveTLSConfig(apiServer configv1.APIServer) (TLSConfigResult, error) {
	profile, err := utiltls.GetTLSProfileSpec(apiServer.Spec.TLSSecurityProfile)
	if err != nil {
		return TLSConfigResult{}, fmt.Errorf("failed to get TLS profile from APIServer %q: %w", apiServer.Name, err)
	}

	return getTLSConfig(apiServer.Spec.TLSAdherence, profile)
}

func getTLSConfig(tlsAdherencePolicy configv1.TLSAdherencePolicy, tlsProfileSpec configv1.TLSProfileSpec) (TLSConfigResult, error) {
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
		// If the cluster-wide TLS adherence policy is not set to honor the cluster-wide TLS profile,
		// use the default TLS profile-based configuration.
		defaultTLSConfig, unsupportedCiphers := utiltls.NewTLSConfigFromProfile(*configv1.TLSProfiles[libgocrypto.DefaultTLSProfileType])
		if len(unsupportedCiphers) > 0 {
			klog.Infof("TLS configuration contains unsupported ciphers that will be ignored: %v", unsupportedCiphers)
		}

		// Set the TLS configuration to the default TLS profile-based configuration.
		tlsConfig = defaultTLSConfig
	}

	return TLSConfigResult{
		TLSConfig:          tlsConfig,
		TLSAdherencePolicy: tlsAdherencePolicy,
		TLSProfileSpec:     tlsProfileSpec,
	}, nil
}
