# ADR-002: Transparent Proxy (iptables) Over Explicit Proxy (HTTP_PROXY)

## Status
Accepted (implementation evolved — see note below)

## Context
We need to route all agent egress through our proxy for policy enforcement and credential injection. Two approaches:

**Explicit proxy:** Set `HTTP_PROXY` / `HTTPS_PROXY` environment variables. Tools that respect these vars (curl, git, npm, pip) will use the proxy.

**Transparent proxy:** Use iptables to redirect ALL outbound TCP at the kernel level. The agent is completely unaware of the proxy.

## Decision
Use transparent proxy with iptables redirect.

## Consequences

**Positive:**
- Agent cannot bypass the proxy (kernel-enforced, not env-var-based)
- Works with ALL tools, including those that ignore HTTP_PROXY
- Agent code doesn't need proxy awareness
- Catches custom HTTP clients, raw TCP connections, and any tool the agent installs
- No configuration needed inside the agent process

**Negative:**
- Requires `NET_ADMIN` capability in the container (for iptables setup)
- More complex proxy implementation (must handle raw TCP, parse TLS ClientHello for SNI)
- Slightly more complex container initialization (iptables rules at startup)
- Non-HTTP TCP traffic needs special handling (passthrough or block)

**Implementation Note (current):**

The original single-container iptables OUTPUT redirect was replaced by a two-container model:
- Agent container: `ip route replace default via $GATEWAY_IP` (all traffic flows to gateway)
- Gateway container: `iptables -t nat -A PREROUTING -p tcp --dport 443 -j REDIRECT --to-port 8443`

The core decision (transparent, kernel-enforced proxy over HTTP_PROXY env vars) remains unchanged. The isolation model is stronger — gateway runs in a separate container the agent cannot access.
