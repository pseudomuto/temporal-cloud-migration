# Cloud gateway playbook and rehearsal: design

## Intent

The current playbook (`index.html`, rehearsed in `rehearsal/`) puts Temporal
Proxy in front of both Temporal Services. Workers and Clients move to the
gateway once, and a handoff removes a routing rule.

Some teams won't put a proxy in front of their self-hosted Temporal Service.
Their apps keep talking straight to it until their Namespace moves, Temporal
Proxy sits in front of Cloud only, and a handoff is a failover followed by a
client flip done in the team's own routing layer (DNS, service discovery,
config). A customer raised this setup along with five questions about it.

This project adds a second playbook and rehearsal for that setup, written for
any team that has it, not for one customer. Where the two setups behave the
same, the new page says the same things as the current one.

## Goals

- A new interactive playbook at `cloud-gateway/index.html`, with the same kit
  as the current one: the details box, live config blocks, the handoff
  tracker, the diagram stepper, per-step checkboxes, and progress saved in
  local storage.
- A new rehearsal at `cloud-gateway/rehearsal/` that runs the new playbook end
  to end on one machine, the way `rehearsal/` runs the current one.
- The page answers the five questions teams with this setup ask (listed
  below), each with a clear answer near the part of the playbook it concerns,
  plus a short section near the top that links to each answer.
- A link at the top of the current playbook pointing to the new one, with one
  sentence on when to use which.

## Non-goals

- The KMS sidecar and `extensionServers`. Both proxies support them
  (`config/extensions.go` in s2s-proxy, `internal/config/extensions.go` in
  Temporal Proxy), but the playbook and rehearsal use static
  `encryption.overrides` with `testing://` keys in the rehearsal, as the
  current pair does. The page says once that the rehearsal checks what the
  keys and labels do, not how a sidecar picks them: if a sidecar picks keys
  differently from the static overrides, the rehearsal won't show it.
- Changing the current playbook or rehearsal beyond the one link.
- The customer's routing layer. The page treats "flip clients" as a step the
  team owns and says what must be true before and after it.
- Answering roadmap questions. Question 2 asks whether per-Namespace
  replication keys are planned; the page describes current behavior only (see
  Open items).

## Anti-goals

- A page that reads as a diff against the current playbook. It stands on its
  own; a reader shouldn't need the other page open.
- Claims the rehearsal hasn't shown. Anything untested goes under "Confirm
  before starting," as it does now.
- Shared code or templating between the two pages. Each page is one
  self-contained HTML file; copy and adapt. Two examples isn't enough to
  abstract.

## How the setup works

Pieces:

- **Self-hosted Temporal Service.** Apps talk to it directly. It stores
  plaintext, for every Namespace, for the whole migration.
- **Temporal Proxy**, in front of Cloud only. One `cloud` upstream with the
  suffix rule, `system` pointing at Cloud, no routing rules, no self-hosted
  upstream, no `bindingLabelsFrom`. Encryption on from the start. Nothing uses
  it until a Namespace's clients flip.
- **s2s-proxy v0.3.1** (same behavior as v0.3.0; that release only lowered the
  minimum Go version) carrying replication, sealing on the way out and opening
  on the way in, as now.

Keys and what they seal:

| Data | Sealed by | Key | Label |
| ---- | --------- | --- | ----- |
| Everything replicated before the Namespace fails over | s2s-proxy | K_repl, shared | Cloud name |
| Worker and Client traffic after the flip | Temporal Proxy | K_ns1 | Cloud name |
| Calls from clients still on self-hosted, forwarded after failover | s2s-proxy | K_ns1 | Cloud name |
| Calls with no Namespace, or a Namespace with no override | Temporal Proxy | K_proxy | as now |

Consequences the page has to spell out, since they differ from the current
playbook:

- K_repl's tail is longer. In the current playbook only history written before
  Phase 1 is under K_repl. Here, everything replicated before each Namespace's
  failover is. Temporal Proxy keeps K_repl in `decryptURIs` until every
  Workflow running at its Namespace's failover has closed and the longest
  retention period has passed since, plus archives.
- Revoking K_ns1 doesn't make that Namespace's pre-failover history
  unreadable. Only revoking K_repl does, and that covers every Namespace.
- Strict is cheaper. Tools on self-hosted see plaintext, so the codec-server
  label mismatch that made strict costly in the current playbook doesn't
  apply. Cross-Namespace traffic still keeps both proxies on permissive.
  Tools reading Cloud directly still need the codec server with the Cloud name
  in the path, or the gateway.
- The Temporal Proxy config doesn't change per handoff. Its live block tracks
  only the Namespace list, overrides, and binding mode. The s2s-proxy block
  tracks the same, plus mappings.
- Before the first handoff, Cloud holds only K_repl payloads. The "labels
  agree" check for K_ns1 can't happen until a Namespace fails over, so the
  first handoff should be a low-risk Namespace and its Verify step includes
  opening a post-failover Workflow in the Cloud UI.

## Playbook structure

Same section layout and visual style as the current page. Per section, what
changes:

- **Header and how-to-use.** Title: "Migrating to Temporal Cloud, with the
  gateway in front of Cloud only." One line linking back to the current
  playbook for teams who route all traffic through Temporal Proxy.
- **Questions this answers.** Short list of the five questions, each linking
  to its answer below.
- **Assumptions, What "no plaintext" covers, How the pieces fit, Keys, Who
  holds what, Namespace binding.** Rewritten for the table above. The diagram
  stepper shows the new phases: apps on self-hosted, Temporal Proxy idle in
  front of Cloud, replication through s2s-proxy, then per-Namespace failover,
  forwarding window, and flip.
- **Phase 0, inventory.** As now, minus moving direct tools to the gateway
  (self-hosted stays plaintext), plus: every app's Workers and Clients must be
  flippable per Namespace. A process that serves several Namespaces through one
  address can't flip one at a time; list those and decide how they move.
- **Phase 1, Temporal Proxy in front of Cloud.** Deploy it, encryption on, no
  traffic yet. Live config block. Smoke test with one Client against a
  throwaway Cloud Namespace.
- **Phase 2, replicate.** As now. Counters before strict:
  `enc_unresolved_namespaces_total`, `enc_unbound_opens_total`,
  `enc_namespace_mismatches_total` at zero; the Cloud UI check opens a
  replicated Workflow (K_repl). Switch to strict before the first handoff.
- **Phase 3, hand off one Namespace.** Tracker columns: Conditions, Failed
  over, Clients flipped, Verified. Fail over first, then flip. The tracker
  flags a flip ticked before failover. A section on the forwarding window:
  what happens to clients still on self-hosted (calls forwarded through
  s2s-proxy, sealed under K_ns1), what to watch, and why to keep it short.
- **Failback.** Fail back, then flip clients back. s2s-proxy opens payloads
  on the way in, including Temporal Proxy's (it holds K_ns1 through its
  override and K_proxy for unwrap), so self-hosted history is plaintext again.
  The tracker unticks right to left, as now.
- **Monitoring.** One section collecting the counters, with what each one
  means and when it must be zero.
- **Phase 4, finish.** Retire s2s-proxy when failback is no longer wanted,
  keep K_repl per the longer tail above, retire it after.
- **Confirm before starting.** Forwarding from a passive Namespace, polls
  included (now the main risk, since every client on self-hosted depends on
  it until its flip); s2s-proxy failback; labels agree across the proxies;
  what a refused replication message does to the stream; the Web UI through
  the gateway.

## The five questions and their answers

1. **Fail over first or flip first?** Fail over first. Either order leans on
   forwarding in one direction. Failing over first leans on the self-hosted
   Temporal Service forwarding to Cloud, which the team configures and the
   rehearsal shows working, polls included; s2s-proxy seals those calls with
   the Namespace's own key from its overrides. Flipping first sends clients
   through Temporal Proxy to a Namespace still passive in Cloud, and works
   only if Cloud forwards to self-hosted; otherwise they get
   `NamespaceNotActive`. The team doesn't control that, and the rehearsal's
   stand-in can't show it (it forwards, under `all-apis-forwarding`). Keep the
   window short, and test it against real Cloud. (Amended 2026-10-29 after the
   rehearsal showed a flip before failover succeeding through the stand-in.)
2. **Per-Namespace replication keys?** Today s2s-proxy seals every admin
   message, replication included, under one replication key; it knows each
   task's Namespace for the label but not for picking a key. With no proxy in
   front of self-hosted, all pre-failover history lands under K_repl. Spell
   out what that means for revocation and retirement. Roadmap: not answered
   on the page.
3. **Permissive first, strict before the first handoff?** Yes, and it costs
   less here than in the current playbook (no direct-tool mismatch). Teams
   with no cross-Namespace traffic can start on strict, but the Phase 2 soak
   still catches unresolved Namespace IDs and missing mappings.
4. **Anything else for failback on the Cloud side?** s2s-proxy needs K_ns1
   and K_proxy (unwrap). Nothing changes in Temporal Proxy. Flip clients back
   first, then fail back: the mirror of "fail over first", leaning only on
   self-hosted forwarding. Failing back first would send gateway clients to a
   Namespace passive in Cloud, depending on Cloud forwarding. Rehearsed for
   payloads under the Namespace's key; K_proxy stays under Confirm before
   starting. (Amended 2026-10-29: Muto chose flip back first after the final
   review found the original order leaned on Cloud forwarding and
   contradicted the tracker's right-to-left untick.)
5. **What to monitor?** The base list, as the customer proposed it and the
   page states it:
   - s2s-proxy: `enc_opaque_rejected_total` and
     `enc_admin_visit_failures_total` (refused data);
     `enc_unresolved_namespaces_total`, `enc_unbound_opens_total`, and
     `enc_namespace_mismatches_total` (must be zero before strict).
   - Temporal Proxy: `vault_ops_total{result="unknown_key"}`,
     `vault_unbound_opens_total`, and `vault_namespace_mismatches_total`.

   Add: replication lag before each failover; Temporal Proxy's
   `server_requests_total` non-OK codes and `router_decisions_total` after
   each flip; `NamespaceNotActive` and forwarding errors on self-hosted during
   the window. The Monitoring section and `bin/metrics` cover the full set.
   The builder confirms `enc_admin_visit_failures_total` exists in s2s-proxy
   v0.3.1 before listing it, and says so on the page if it doesn't.

## Rehearsal

Same base as `rehearsal/`: both local Temporal Services on sqlite, both
s2s-proxies (`temporalio/s2s-proxy:v0.3.1`), `testing://` keys, Temporal
Proxy on the host from source, the rehearsal Worker, and adapted copies of
`bin/gen-config`, `bin/payloads`, and `bin/metrics`. Differences:

- Temporal Proxy has one upstream (`cloud`) and `system` points there.
  `gen-config` drops `HANDED_OFF` and the rules, since handoff no longer
  touches the config.
- The Worker starts pointed straight at self-hosted (`localhost:17233`,
  Namespace `ns1`). The flip is restarting it at the gateway
  (`localhost:7233`). The page says this stands in for the team's routing
  layer.
- Phase 3 holds the forwarding window open on purpose: after failover, with
  the Worker still on self-hosted, start Workflows and send signals, then use
  `bin/payloads` to show those calls in Cloud history under K_ns1 with the
  Cloud label. Same caveat as now: this shows how the self-hosted Temporal
  Service forwards under the `dcRedirectionPolicy` given, not how Cloud does.
- `bin/payloads` on Cloud history shows the three cases: K_repl before
  failover, K_ns1 after (forwarded and via the gateway), all labelled
  `ns1.acct`. On self-hosted it shows plaintext before failover and after
  failback.
- Failback: fail back, flip the Worker back, show self-hosted history is
  plaintext again, including events Temporal Proxy sealed.
- Three Web UIs (my call): one on self-hosted directly (plaintext, no codec
  server), one on the gateway, and `ui-cloud` straight at the cloud stand-in
  with Temporal Proxy's codec server, as now. `ui-cloud` is the only way to do
  the Cloud UI check: before the flip, the gateway UI reading a Namespace
  that's passive in cloud gets forwarded to self-hosted and shows that copy.
  So the generated Temporal Proxy config keeps the codec server.

## Left to the builder

- Copy and adapt both current pages and scripts; keep their idiom, comment
  density, and style.
- Exact wording, diagram layout, and which counters `bin/metrics` prints
  beyond those named here.
- Ports for the rehearsal; reuse the current ones so both can't run at once,
  and say so.
- Run the rehearsal end to end before calling it done, and record anything
  that contradicts this design.

## Open items

- Question 2's roadmap part (are per-Namespace replication keys planned?) is
  for the s2s-proxy owners, not the page.
