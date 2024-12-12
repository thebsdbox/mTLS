# The DTunnel (Dan Tunnel)

This is an attempt at moving away from Sidecars/iptables/nftables and having everything managed by a single instance per node, an original idea.

The original design consisted of:
- Proxy and eBPF (sidecar)
- Controller and Watcher (single instance)

The new design will consiste of:
- Single daemonset, with eBPF

## Dtunnel architecture per node

### eBPF

This will work in the same manner, and will watch for all `connect()` operations and if they're in the pod CIDR will redirect them on to the per-node proxy tunnels podIP.

### Proxy tunnel

This will be listening on it's podIP (localhost wont work), where traffic will be directed to. We will then use the `SO_REDIRECT` to find the original destination, where we then will use the Kubernetes API to look up where this pod IP is living. We set the *new* destination to that host and the proxy IP.