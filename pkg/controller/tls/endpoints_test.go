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
	"testing"

	"sigs.k8s.io/controller-runtime/pkg/webhook"
)

func TestCoveredTLSEndpoints(t *testing.T) {
	endpoints := CoveredTLSEndpoints()
	if len(endpoints) != 6 {
		t.Fatalf("expected 6 FR-011 endpoints, got %d", len(endpoints))
	}

	wantPorts := map[string]int{
		endpointOperatorMetrics:      8443,
		endpointSpireServerGRPC:      8081,
		endpointSpireFederationHTTPS: 8443,
		endpointSpireServerMetrics:   8082,
		endpointOIDCDiscoveryHTTPS:   8443,
		endpointSpireCtrlMgrWebhook:  9443,
	}

	seen := make(map[string]struct{}, len(endpoints))
	for _, ep := range endpoints {
		if ep.ID == "" {
			t.Fatal("endpoint ID must not be empty")
		}
		if _, dup := seen[ep.ID]; dup {
			t.Fatalf("duplicate endpoint ID %q", ep.ID)
		}
		seen[ep.ID] = struct{}{}

		wantPort, ok := wantPorts[ep.ID]
		if !ok {
			t.Fatalf("unexpected endpoint ID %q", ep.ID)
		}
		if ep.Port != wantPort {
			t.Fatalf("endpoint %q: expected port %d, got %d", ep.ID, wantPort, ep.Port)
		}
	}

	metrics := endpoints[3]
	if len(metrics.AlternatePorts) != 1 || metrics.AlternatePorts[0] != 9402 {
		t.Fatalf("spire-server-metrics alternate ports: want [9402], got %v", metrics.AlternatePorts)
	}
}

func TestOperatorWebhookPortMatchesControllerRuntimeDefault(t *testing.T) {
	if OperatorWebhookPort != webhook.DefaultPort {
		t.Fatalf("OperatorWebhookPort=%d, controller-runtime DefaultPort=%d", OperatorWebhookPort, webhook.DefaultPort)
	}
	if OperatorWebhookPort != 9443 {
		t.Fatalf("expected operator webhook port 9443, got %d", OperatorWebhookPort)
	}
}
