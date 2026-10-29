# Cloud gateway rehearsal run

Run of `cloud-gateway/rehearsal/` on 2026-10-29, to back the claims in the
cloud gateway playbook and rehearsal guide. Versions: `temporalio/server:1.32.1`,
`temporalio/s2s-proxy:v0.3.1`, Temporal Proxy v1.0.1, Temporal CLI 1.9.1.

One Worker process per Namespace (`NAMESPACES=nsN`), so each flips on its own.
A flip is restarting that Worker with `TEMPORAL_ADDRESS=localhost:7233`.

## Facts

### Setup and replication

- `bin/gen-config` writes a Temporal Proxy config with no `self-hosted`
  mention, `default: cloud`, `system: cloud`; Temporal Proxy v1.0.1 starts on
  it (gateway, codec server, and metrics listeners up).
- Compose project needed its own name: both rehearsal directories are named
  `rehearsal`, so `up` in one would replace the other's containers.
- Before replication, `bin/payloads self-hosted ns1 pre-1` shows every payload
  `json/plain`, key `-`, label `-`.
- After adding cloud as a replica and force-replicating,
  `bin/payloads cloud ns1.acct pre-1` shows every payload
  `binary/encrypted`, key `repl`, label `ns1.acct`.
- `post-repl-1`, started on self-hosted after the replica existed: self-hosted
  has `json/plain`; cloud has `repl` / `ns1.acct`. So everything replicated
  before failover is under K_repl, not only history from before replication.
- Codec server (what `ui-cloud` calls), permissive: a `pre-1` payload from
  cloud decodes to `"secret before replication"` with `X-Namespace: ns1.acct`,
  and also with `X-Namespace: ns1`.
- Temporal Proxy's request counter is `tmprl_proxy_server_requests_total`
  (labels `code`, `method`).
- Before any flip, Temporal Proxy's only requests are the gateway UI's health
  watch; `router_decisions_total` is all zeros.

### Strict

- `BINDING=strict bin/gen-config`, restart s2s-self-hosted and Temporal Proxy.
  Codec server: `X-Namespace: ns1.acct` gives HTTP 200, `X-Namespace: ns1`
  gives HTTP 400 and `vault_ops_total{result="mismatch"}` +1. Under permissive
  the `ns1` decode succeeded and counted
  `vault_namespace_mismatches_total{namespace="ns1"}` 1.
- Signal to `pre-1` on self-hosted under strict replicates to cloud as
  `repl` / `ns1.acct`. s2s-proxy's `enc_*` counters stay absent (zero).

### Forwarding window (ns1 failed over, its Worker still on self-hosted)

- `temporal operator namespace update --address localhost:17233 -n ns1
  --active-cluster cloud`; cloud reports `ns1.acct` active on `cloud`.
- With `dcRedirectionPolicy: all-apis-forwarding` on self-hosted: a
  `workflow start` (`window-1`) and a `signal` (`pre-1`) sent to self-hosted
  succeed, and ns1's Worker, still polling self-hosted, runs both Workflows'
  tasks. Polls are forwarded. Worker log: no errors or warnings.
- Cloud history: `window-1` all `ns:ns1` / `ns1.acct`; `pre-1` events after
  failover `ns:ns1` / `ns1.acct`, earlier ones `repl`.
- Self-hosted's passive copy of `window-1` is `json/plain`: s2s-proxy opens
  replication coming back in. The Worker on self-hosted got plaintext through
  forwarded polls (it ran the Echo Activity on the inputs).
- This is the self-hosted Temporal Service forwarding to the cloud stand-in,
  under the policy above. It says nothing about how real Cloud handles calls,
  and it is one run with light traffic.

### Flip (ns1's Worker restarted at the gateway)

- Through the gateway: `workflow start` (`flip-1`) and two signals to `pre-1`
  (`ping`, `fail`) succeed; the Worker runs them; only Worker errors are the
  requested `Fail` Activity.
- Cloud history: `flip-1` all `ns:ns1` / `ns1.acct`. `pre-1` now holds 30
  `repl` / `ns1.acct` payloads and 19 `ns:ns1` / `ns1.acct`; every label is
  `ns1.acct`.
- Through the gateway, `pre-1`'s first payload (sealed under `repl`) reads
  `"secret before replication"`, and the failure message reads
  `rehearsal failure: card number 4111`; in cloud directly it is
  `Encoded failure`.
- Temporal Proxy: `vault_ops_total{result="success"}` for decrypt and encrypt,
  no `unknown_key`, no new mismatch or unbound (the one mismatch is the
  deliberate `X-Namespace: ns1` codec call). Requests count under
  `router_decisions_total{outcome="default",upstream="cloud"}` (no rules, so
  `default`, not `match`) and `{outcome="system"}` for `GetSystemInfo`. All
  `server_requests_total` codes `OK` (plus the UI health watch `Canceled`).

### Failback (ns2: failed over, flipped, then failed back and flipped back)

- After failover and flip, a signal through the gateway lands in cloud under
  `ns:ns2` / `ns2.acct` (cloud `pre-1`: 43 `repl`, 5 `ns:ns2`).
- `temporal operator namespace update --address localhost:27233 -n ns2.acct
  --active-cluster self-hosted`; self-hosted reports `ns2` active on
  `self-hosted`. ns2's Worker restarted at `localhost:17233`; a signal there
  succeeds and the Worker runs it with no errors.
- Self-hosted `ns2 pre-1`: all 55 payloads `json/plain`, including the
  `"written on cloud"` signal Temporal Proxy sealed under `ns:ns2`. s2s-proxy
  opened it on the way in with the Namespace's key from its overrides.
- Events written on self-hosted after failback replicate to cloud under `repl`
  again (cloud `pre-1`: 48 `repl`, 7 `ns:ns2`).
- No `unknown_key`, unbound, or new mismatch on either proxy; s2s-proxy
  `enc_*` counters absent throughout the run, so
  `enc_admin_visit_failures_total` and `enc_opaque_rejected_total` were never
  incremented and their exported names weren't observed.

### Failback, flip back first (third run, after review)

- Fresh stack, strict. ns2 failed over, Worker flipped to the gateway, a
  signal written on cloud (`ns:ns2`). Then ns2's Worker restarted at
  `localhost:17233` while ns2 was still active on cloud: a signal sent to
  self-hosted was forwarded, landed in cloud under `ns:ns2` / `ns2.acct`, and
  the Worker ran it.
- Then `--active-cluster self-hosted` on `ns2.acct`, and a signal to
  self-hosted. Self-hosted `ns2 pre-1`: all 14 payloads `json/plain`, all
  three signals readable. No Worker errors; no `unknown_key`, mismatch, or
  unbound; s2s-proxy `enc_*` counters absent.
- So flip back, then fail back, works and leans only on self-hosted
  forwarding (the same direction as the handoff window).

### Flip before failover (second run, ns3 still active on self-hosted)

- Fresh stack, strict, ns3 replicated but not failed over. Through the gateway:
  `temporal workflow start -n ns3 -w early-flip` succeeds; no
  `NamespaceNotActive`.
- The `cloud` stand-in (`dcRedirectionPolicy: all-apis-forwarding` in
  `temporal/cloud.yaml`) forwarded it to self-hosted, where ns3 is active.
  Self-hosted stores `early-flip` as `json/plain` (Temporal Proxy sealed it,
  s2s-proxy opened it on the way in); cloud's replica has it under `repl` /
  `ns3.acct`. ns3's Worker, still on self-hosted, runs it.
- So in this rehearsal either order works, each leaning on forwarding in one
  direction. Whether real Cloud forwards calls for a passive Namespace back to
  self-hosted is not shown here.

- Second run: a `flip-1` payload in cloud (`ns:ns1` / `ns1.acct`) decodes
  through the codec server with `X-Namespace: ns1.acct` to
  `"secret after flip"`, HTTP 200, under strict.
- Headless Chrome (`--dump-dom --virtual-time-budget`) never returns on the
  Web UI's Workflow pages, so the UIs were not checked automatically.

- Not from the run, from source: s2s-proxy v0.3.1 defines
  `enc_admin_visit_failures_total` ("admin calls and streams failed because a
  message could not be sealed or opened, whatever the cause, labeled by
  method") and `enc_opaque_rejected_total` (labeled by kind) in
  `metrics/prometheus_defs.go` (lines 88 and 89). Its other `enc_*` counters
  were exported with the `temporal_s2s_proxy_` prefix on the live endpoint.

### Not shown by this run

- K_proxy on the way back: every Namespace had an override, so nothing was
  sealed under K_proxy, and s2s-proxy opening K_proxy payloads on failback
  wasn't exercised.
- How real Cloud forwards calls from a passive Namespace, and forwarding under
  load or for long polls held open across the failover.
- A refused replication message (no opaque data was produced).
- The Web UIs were not driven in a browser; the `ui-cloud` check was done by
  calling the codec server it uses.

## Theories

- The window after failover depends on `dcRedirectionPolicy`. A policy that
  doesn't forward polls would leave Workers on self-hosted idle until their
  flip. Untested here; the stand-ins both use `all-apis-forwarding`.
