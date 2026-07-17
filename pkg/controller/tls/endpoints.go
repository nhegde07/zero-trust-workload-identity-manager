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

import "sigs.k8s.io/controller-runtime/pkg/webhook"

// TLSEndpoint describes a TLS-serving server endpoint covered by FR-011.
type TLSEndpoint struct {
	// ID is a stable identifier used in runbooks and verification matrices.
	ID string
	// Description is a human-readable label for the endpoint.
	Description string
	// Port is the primary TCP port used for TLS scanning.
	Port int
	// AlternatePorts lists additional ports for the same logical endpoint when applicable.
	AlternatePorts []int
	// Workload identifies the Kubernetes workload that serves the endpoint.
	Workload string
}

const (
	endpointOperatorMetrics      = "operator-metrics"
	endpointSpireServerGRPC      = "spire-server-grpc"
	endpointSpireFederationHTTPS = "spire-federation-https"
	endpointSpireServerMetrics   = "spire-server-metrics"
	endpointOIDCDiscoveryHTTPS   = "oidc-discovery-https"
	endpointSpireCtrlMgrWebhook  = "spire-controller-manager-webhook"
)

// OperatorWebhookPort is the controller-runtime default admission webhook listen port.
var OperatorWebhookPort = webhook.DefaultPort

// CoveredTLSEndpoints returns the six FR-011 TLS-serving endpoints in scan order.
func CoveredTLSEndpoints() []TLSEndpoint {
	return []TLSEndpoint{
		{
			ID:          endpointOperatorMetrics,
			Description: "Operator metrics HTTPS",
			Port:        8443,
			Workload:    "zero-trust-workload-identity-manager controller-manager",
		},
		{
			ID:          endpointSpireServerGRPC,
			Description: "SPIRE server registration API (gRPC over TLS)",
			Port:        8081,
			Workload:    "spire-server",
		},
		{
			ID:          endpointSpireFederationHTTPS,
			Description: "SPIRE federation bundle HTTPS",
			Port:        8443,
			Workload:    "spire-server",
		},
		{
			ID:             endpointSpireServerMetrics,
			Description:    "SPIRE server metrics HTTPS",
			Port:           8082,
			AlternatePorts: []int{9402},
			Workload:       "spire-server / spire-controller-manager sidecar",
		},
		{
			ID:          endpointOIDCDiscoveryHTTPS,
			Description: "OIDC discovery provider HTTPS",
			Port:        8443,
			Workload:    "spire-oidc-discovery-provider",
		},
		{
			ID:          endpointSpireCtrlMgrWebhook,
			Description: "SPIRE controller-manager admission webhook HTTPS",
			Port:        9443,
			Workload:    "spire-controller-manager",
		},
	}
}
