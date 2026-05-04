SCC Hardening for SPIRE Agent and SPIFFE CSI Driver

## *Proposed SCC hardening strategy for ZTWIM operands to enforce least privilege while maintaining strict control over granted permissions.*

**Date:** 		Apr 9, 2026  
**Scope:**		Zero Trust Workload Identity Manager (ZTWIM) Operator  
**Status:** 		Proposed  **Status Changed Date:** Apr 9, 2026  
**Authors:** 		[Nandan Hegde](mailto:nandan.islur@gmail.com)  
**Other docs:**

- [SCC Hardening Review \+ Spike Guide](https://docs.google.com/document/d/1uM8NCAp_0SurCpuM_G9N58lZV4e1I35Gj_obd1-Lreg/edit?tab=t.rbqyrsmm2gpz#heading=h.d9cvdryh0yhe)

# What

SPIRE Agent and SPIFFE CSI Driver currently run in privileged mode using custom SCCs. This introduces unnecessary security exposure, particularly for the SPIRE Agent where privileged mode is not functionally required.

This ADR proposes:

1. Removing privileged mode from SPIRE Agent and running it as root with all capabilities dropped  
2. Retaining privileged mode for SPIFFE CSI Driver due to *bidirectional mount* propagation requirements  
3. Standardizing on custom SCCs for both SPIRE Agent and SPIFFE CSI Driver to ensure tightly scoped permissions and avoid reliance on overly permissive default SCCs

# 

# Why

### **Problem**

Both SPIRE Agent and SPIFFE CSI Driver currently run as privileged containers. While required for SPIFFE CSI Driver, this is unnecessary and overly permissive for SPIRE Agent, increasing the attack surface.

### **Root Cause**

* SPIRE Agent privileged requirement was historically used to bypass filesystem permission constraints for socket creation  
* SPIFFE CSI Driver requires **bidirectional mount propagation**, which Kubernetes allows only for privileged containers  
* No existing OpenShift SCC provides a minimal, least-privileged configuration matching these requirements

### **Key Findings**

* SPIRE Agent requires:  
  * Root equivalent user (UID 0\) to create UDS socket file  
    * POSIX permissions are sufficient for Unix Domain Socket creation  
  * No additional Linux capabilities  
    * Privileged mode grants all capabilities (not limited by drop rules) and thus breaking container isolation  
* SPIRE-CSI-DRIVER requires:  
  * Privileged mode to allow bidirectional hostpath mount. This is the only way possible for kubelet to inform CSI driver through NodePublishVolume calls

### **Conclusion**

* Spire agent works with required capabilities by inheriting the user defined in dockerfile, that means, there will be no explicit privilege mode enabled  
* Privileged mode is **required** for CSI driver to perform bidirectional mounting  
* Custom SCCs are required for both components to enforce least privilege and avoid broader system-level exposure

## 

## Goals

- Eliminate unnecessary privileged access for SPIRE Agent  
- Enforce least privilege principle across operands  
- Maintain full functional correctness  
- Avoid introducing breaking changes to workloads

## Non-Goals

- Redesign of SPIFFE CSI driver architecture to avoid privilege permission  
- Removal of hostPath or PID-based attestation mechanisms in SPIRE Agent  
- Adding required capability permissions to non-root user

# How

## SPIRE Agent SCC Hardening

### Daemonset (podSpec) changes

* *podspec.securityContext.capabilities: drop all*  
* *podspec.securityContext.Privileged: false*  
* *podspec.networkPolicy : clusterFirst*   
  * We do not need hostNetwork since MY\_NODE\_NAME is set in podspec  
* *podspec.hostNetwork: false*  
* Proposed Daemonset for SPIRE-Agent

```
apiVersion: apps/v1
kind: DaemonSet
metadata:
  name: spire-agent
  # ...
spec:
  # ...
    spec:
      hostPID: true          # workload attestation; SCC must allow host PID
      hostNetwork: false
      dnsPolicy: ClusterFirst
      serviceAccountName: spire-agent
      containers:
        - name: spire-agent
          image: <spire-agent-image>
          # probes, ports, volumeMounts, resources...
          securityContext:
            allowPrivilegeEscalation: false
            privileged: false
            capabilities:
              drop: ["ALL"]
            readOnlyRootFilesystem: true
```


### SCC changes

* *hostNetwork* and *hostPort* permission are denied  
* *runAsUser* is set to *runAsAny* to facilitate SPIRE Agent to run as root user (default user is UID 0 in the SPIRE Agent docker file)  
* Custom SPIRE Agent SCC without any RBAC to the operator.  
* Proposed SCC for SPIRE Agent

```
apiVersion: security.openshift.io/v1
kind: SecurityContextConstraints
metadata:
  name: spire-agent
  # ...
readOnlyRootFilesystem: true
runAsUser:
  type: RunAsAny
seLinuxContext:
  type: MustRunAs
supplementalGroups:
  type: MustRunAs
fsGroup:
  type: MustRunAs
users:
  - system:serviceaccount:<operator-namespace>:spire-agent
groups: []
allowHostDirVolumePlugin: true
allowHostIPC: false
allowHostNetwork: false
allowHostPID: true
allowHostPorts: false
allowPrivilegeEscalation: false
allowPrivilegedContainer: false
allowedCapabilities: []
defaultAddCapabilities: []
requiredDropCapabilities:
  - ALL
volumes:
  - configMap
  - hostPath
  - projected
  - secret
  - emptyDir
```

### Rationale

* Root is required for creating and binding to socket file in hostPath-mounted directory  
  * Backing node volume is created by kubelet and it’s root owned. Note that only POSIX file permissions are sufficient to create and bind UDS socket file  
* Additional Capabilities or Privilege mode are not required for socket creation.  
  * A privileged container can do almost anything the host can do (like accessing hardware or reconfiguring the kernel), making it a massive security risk, hence by not allowing the container to run in privileged mode and dropping all capabilities.  
* Disable hostNetwork and hostPort (hostNetwork is not necessary when MY\_NODE\_NAME is set) and set podspec.DNSpolicy to ClusterFirst  
  * When the above mentioned env is unset, only then the agent needs hostNetwork permission to talk to kubelet. Unnecessary permission may lead to increasing attack surface  
* For proposing custom SPIRE Agent SCC  
  * Currently there doesn’t exist a minimum permission SCC which matches spire agent requirement. Alternate will be to use a higher privileged system SCC which makes the agent security susceptible to a broader set of permissions than necessary.

## SPIRE CSI Driver SCC Hardening

### Daemonset (podSpec)

* Remains privileged (no change)

### SCC changes

* No changes. We continue to use custom SCC for SPIRE-CSI-Driver 

### Rationale

* Privilege permission is necessary for CSI Driver to allow Bidirectional mount propagation  
  * Csi-node-registrar uses hostPath mounting to register the SPIRE-CSI-Driver. Kubelet invokes *NodePublishVolume* grpc invocation of SPIRE-CSI-Driver when any workload uses csi driver. This is only available via bidirectional mounting and hence privileged.  
* Custom SCC for SPIRE-CSI-Driver  
  * Custom SCC fits here because it will allow restricting ztwim operator service account on role binding permissions. Alternate would be to use system:privileged SCC which would mean to give broader permission to ztwim operator to enable rolebinding to system:privileged SCC.

## Alternatives

1. Keep SPIRE Agent privileged  
   * Rejected: violates least privilege and introduces security risk  
2. Run SPIRE Agent as non-root without privileged  
   * Rejected: fails due to inability to create socket in root-owned directory since crio doesn’t allow to grant ambient capabilities to the non root user. Ambient capabilities are necessary for a non root user to grant capabilities. This is a shortcoming in both k8s.  
3. Use non-root \+ privileged \+ drop capabilities  
   * Rejected: ineffective as privileged mode restores all capabilities and these capabilities give near system admin control to the host node  
4. Use default hostmount-anyuid SCC for SPIRE Agent  
   * Rejected: This SCC doesn't grant HostPID permission which is necessary for agent.  
5. Use default hostaccess SCC for SPIRE Agent  
   * Rejected: This SCC doesn't let agent to run as root user.  
6. Use *non root user without privilege* for SPIRE-CSI-DRIVER  
   * Rejected: kubernetes doesn't allow the pod to be non privileged and be bidirectional mounting at the same time.

## Risks

| Risk | Business Impact | Mitigation |
| :---- | :---- | :---- |
| Privileged mode for CSI Driver | High Security Impact | Currently this is the functionality requirement. We can think of coming up with a document on what risks can be associated with privilege mode |
| Root user for SPIRE Agent | High Security Impact | We have dropped the privilege to ensure that the root user is just restricted to POSIX file permission when it comes to executing the binary. Enforcement of ReadOnlyRFS is in place. Root mode is still better compared to non-root+privileged in terms of capability being used in container runtime. |
| HostPath Mounting | High Security Impact | SPIRE agent and CSI Driver both are designed to use HostPath Mounting. Once again, this is essential for functionality. |
| Namespace PSA escalation | High Security Impact | Document the behaviour and isolate the namespace in future if needed. |

# Reviews

Anybody may review the document and provide feedback.  Acceptance and rejection is reserved for those people noted in the appropriate "Accept / Reject" section of each ADR.

| Reviewed by | Date  | Notes |
| :---- | :---- | :---- |
| [Raushan Kumar Singh](mailto:rausingh@redhat.com) | Apr 12, 2026 | Initial review done. Added a few comments. |
| [Trilok Geer](mailto:tgeer@redhat.com) | Apr 17, 2026 | Completed review and approved |

