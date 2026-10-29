# Cloud gateway playbook and rehearsal implementation plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** A second interactive playbook and local rehearsal for teams whose apps talk straight to self-hosted Temporal until their Namespace moves, with Temporal Proxy in front of Cloud only.

**Architecture:** A new `cloud-gateway/` directory beside the current playbook, holding a self-contained `index.html` (copied from `index.html` and adapted) and a `rehearsal/` directory (copied from `rehearsal/` and adapted). The rehearsal is built and run first, so the pages state only what it showed.

**Tech Stack:** Static HTML/CSS/vanilla JS, bash, Docker Compose, `temporalio/server:1.32.1`, `temporalio/s2s-proxy:v0.3.1`, `temporalio/ui:2.56.0`, Temporal Proxy `go.temporal.io/proxy/cmd/proxy@v1.0.1`, Temporal CLI 1.9.1, Go 1.27.

**Spec:** `docs/superpowers/specs/2026-10-29-cloud-gateway-playbook-design.md`

## Global constraints

- No emojis, no em-dashes, no en-dashes in anything written (pages, comments, scripts, commits). Sentence-case headings.
- Do not change `index.html` or `rehearsal/` except the one link added in Task 6.
- Each page is one self-contained HTML file. No shared JS/CSS between the two playbooks. Copy and adapt.
- s2s-proxy image: `temporalio/s2s-proxy:v0.3.1`. Verified published; release only lowered the minimum Go version, so behavior matches v0.3.0.
- Rehearsal keys are `testing://` under static `encryption.overrides`. No KMS sidecar, no `extensionServers`. The rehearsal page says once that it tests keys and labels, not how a sidecar picks keys.
- Rehearsal reuses the current rehearsal's ports, so the two rehearsals can't run at once. Say so on the rehearsal page.
- Cloud names are the local name plus `.{acct}`; the rehearsal uses `acct`.
- Anything not shown by the Task 2 run goes under "Confirm before starting," never stated as fact.
- `enc_admin_visit_failures_total` is verified to exist (`metrics/prometheus_defs.go` in s2s-proxy).
- Commits: `git add` files individually, never `git add .`. Ask Muto for each commit's subject line before committing.

## Review focus

1. **A flip ticked before failover in the tracker.** Expect a warning in that row naming `NamespaceNotActive`, and the next-up hint telling the reader to fail over first. (Task 4)
2. **Unticking a failback right to left.** Expect the row to go back to "self-hosted" with no stale warnings, and the diagram to show clients back on self-hosted. (Task 4)
3. **Empty or odd details-box input** (no Namespaces, whitespace, account left blank). Expect the live blocks to render with `<account>` placeholders and no JS errors, as the current page does. (Task 4)
4. **The rehearsal Worker for a flipped Namespace still running against self-hosted.** Expect forwarded calls in Cloud history under `ns:ns1` labelled `ns1.acct`, and the page to say only what Task 2 recorded. (Tasks 2 and 3)
5. **Rerunning `bin/gen-config` after strict, or across a reset.** Expect saved settings to be reused and keys never regenerated while `generated/keys` exists. (Task 1)

---

### Task 1: rehearsal scaffolding

**Files:**
- Create: `cloud-gateway/rehearsal/.gitignore` (copy of `rehearsal/.gitignore`)
- Create: `cloud-gateway/rehearsal/mise.toml` (copy)
- Create: `cloud-gateway/rehearsal/docker-compose.yaml`
- Create: `cloud-gateway/rehearsal/bin/gen-config`
- Create: `cloud-gateway/rehearsal/bin/payloads` (copy, header comment adapted)
- Create: `cloud-gateway/rehearsal/bin/metrics`
- Create: `cloud-gateway/rehearsal/temporal/` (copies of `rehearsal/temporal/*.yaml`, unchanged)
- Create: `cloud-gateway/rehearsal/worker/` (copy of `rehearsal/worker/`, unchanged; it already takes `TEMPORAL_ADDRESS` and `NAMESPACES`)

**Interfaces:**
- Produces: `bin/gen-config` settings `ACCT`, `NAMESPACES`, `BINDING`, `ALLOW_OPAQUE_PLAINTEXT` (no `HANDED_OFF`), remembered in `generated/settings`. Writes `generated/temporal-proxy.yaml`, `generated/s2s-self-hosted.yaml`, `generated/s2s-cloud.yaml`, `generated/keys` (names `proxy`, `repl`, `ns:<ns>`).
- Produces: `bin/payloads self-hosted|cloud NAMESPACE WORKFLOW_ID` and `bin/metrics`, same output shape as the current scripts.
- Produces: compose services `temporal-self-hosted` (17233), `temporal-cloud` (27233), `s2s-self-hosted` (metrics 19091), `s2s-cloud` (29091), `ui-self-hosted` (8082, `TEMPORAL_ADDRESS: temporal-self-hosted:7233`, no codec), `ui-gateway` (8080), `ui-cloud` (8081, codec `http://localhost:8445`).

- [ ] **Step 1: Write `bin/gen-config`.** Same structure as `rehearsal/bin/gen-config`, with these changes to `generated/temporal-proxy.yaml`: no `x-self-hosted` anchor, no `rules`, no `self-hosted` upstream; `routing.default: cloud` and `routing.system: cloud`; the `cloud` upstream keeps `hostPort: localhost:27233`, `insecure: true`, `suffix: .$acct`. Encryption, codec server, and metrics blocks unchanged. The s2s configs are unchanged except the header comment. Drop `HANDED_OFF` everywhere, including validation and `generated/settings`. The final echo prints the binding only.

- [ ] **Step 2: Run it and check the output.**

Run: `cd cloud-gateway/rehearsal && bin/gen-config && grep -c 'self-hosted' generated/temporal-proxy.yaml; grep -E '^\s+(default|system):' generated/temporal-proxy.yaml; awk '{print $1}' generated/keys`
Expected: `0`; `default: cloud` and `system: cloud`; `proxy`, `repl`, `ns:ns1`, `ns:ns2`, `ns:ns3`.

- [ ] **Step 3: Check that settings persist and keys don't regenerate (review focus 5).**

Run: `sha=$(shasum generated/keys); BINDING=strict bin/gen-config; bin/gen-config; grep namespaceBinding generated/*.yaml; [ "$sha" = "$(shasum generated/keys)" ] && echo keys-stable`
Expected: every `namespaceBinding: strict`, then `keys-stable`.

- [ ] **Step 4: Write `docker-compose.yaml`** from the current one, s2s image default `v0.3.1`, plus the `ui-self-hosted` service above with a comment saying it shows plaintext because self-hosted stores plaintext. Point `bin/metrics` at the same counters as now plus `enc_admin_visit_failures_total` on s2s-proxy and `server_requests_total` on Temporal Proxy (check the exact prefixed name on the live endpoint in Step 6).

- [ ] **Step 5: Bring it up.**

Run: `BINDING=permissive bin/gen-config && docker compose up -d --wait && docker compose ps --format '{{.Service}} {{.Health}}'`
Expected: both Temporal Services and both s2s-proxies `healthy`; UIs running.

- [ ] **Step 6: Start Temporal Proxy and read metrics.**

Run (second terminal): `go run go.temporal.io/proxy/cmd/proxy@v1.0.1 serve -c generated/temporal-proxy.yaml`, then `bin/metrics`.
Expected: proxy starts with no config error; `bin/metrics` reaches both endpoints.

- [ ] **Step 7: Commit** (ask Muto for the subject).

```bash
git add cloud-gateway/rehearsal/.gitignore cloud-gateway/rehearsal/mise.toml cloud-gateway/rehearsal/docker-compose.yaml cloud-gateway/rehearsal/bin/gen-config cloud-gateway/rehearsal/bin/payloads cloud-gateway/rehearsal/bin/metrics cloud-gateway/rehearsal/temporal/cloud.yaml cloud-gateway/rehearsal/temporal/self-hosted.yaml cloud-gateway/rehearsal/temporal/dynamic-config.yaml cloud-gateway/rehearsal/worker/go.mod cloud-gateway/rehearsal/worker/go.sum cloud-gateway/rehearsal/worker/main.go
```

### Task 2: run the rehearsal end to end and record what happened

**Files:**
- Create: `investigations/cloud-gateway-rehearsal.md`. FACTS (command plus the output line that shows it) kept apart from THEORIES.

**Interfaces:**
- Consumes: everything from Task 1.
- Produces: the record Tasks 3 to 5 cite. Each claim on either page must trace to a FACT here or go under "Confirm before starting."

Follow the current rehearsal guide's commands (`rehearsal/index.html`, Set up through Roll back), with these changes. Run one Worker process per Namespace: `NAMESPACES=ns1 TEMPORAL_ADDRESS=localhost:17233 go run .` and likewise for ns2 and ns3, so each Namespace flips on its own.

- [ ] **Step 1: Phase 0 to 2.** Connect clusters, register Namespaces, start Workflows `pre-1` before replication, replicate, start `post-repl-1` after. Record: `bin/payloads self-hosted ns1 pre-1` shows plaintext (`json/plain`, key `-`); `bin/payloads cloud ns1.acct pre-1` shows `repl` labelled `ns1.acct`; `ui-cloud` at 8081 opens `pre-1` in `ns1.acct`; `ui-self-hosted` at 8082 shows plaintext.
- [ ] **Step 2: Strict.** `BINDING=strict bin/gen-config`, restart s2s-self-hosted and Temporal Proxy, signal `pre-1`, record `bin/metrics` (all mismatch/unbound/unresolved counters absent or zero).
- [ ] **Step 3: Fail over ns1, keep its Worker on self-hosted (the forwarding window).** `temporal operator namespace update --address localhost:17233 -n ns1 --active-cluster cloud`. Then via self-hosted directly: start `window-1`, signal `pre-1`. Record: do the Workflows make progress with the Worker still polling self-hosted (polls forwarded or not), and `bin/payloads cloud ns1.acct window-1` shows `ns:ns1` labelled `ns1.acct`. Expected if forwarding works: yes and yes. If not, record the exact errors; that becomes a Confirm-before-starting item, not a fact.
- [ ] **Step 4: Flip ns1.** Restart ns1's Worker with `TEMPORAL_ADDRESS=localhost:7233`. Start `flip-1` through the gateway, signal `pre-1` and send `fail`. Record: `bin/payloads cloud ns1.acct flip-1` shows `ns:ns1`/`ns1.acct`; `pre-1` history in Cloud mixes `repl` (before failover) and `ns:ns1` (after); `ui-gateway` shows `pre-1` plaintext; `bin/metrics` shows no `unknown_key`, mismatch, or unbound.
- [ ] **Step 5: Fail back ns2 after handing it off.** Fail over ns2, flip, signal, then `temporal operator namespace update --address localhost:27233 -n ns2.acct --active-cluster self-hosted`, flip ns2's Worker back to 17233, signal. Record: `bin/payloads self-hosted ns2 pre-1` shows every payload plaintext, including events Temporal Proxy sealed.
- [ ] **Step 6: Gate.** If any result contradicts the spec's "How the setup works" section, STOP and report to Muto with the command and output before Task 3.
- [ ] **Step 7: Commit** `investigations/cloud-gateway-rehearsal.md` (ask Muto for the subject). Tear down with `docker compose down`.

### Task 3: rehearsal guide page

**Files:**
- Create: `cloud-gateway/rehearsal/index.html`, adapted from `rehearsal/index.html` (same CSS, layout, `$`-prefixed commands, GitHub source links rewritten to `cloud-gateway/rehearsal/...`).

**Interfaces:**
- Consumes: commands and outputs from Task 2's record.

- [ ] **Step 1: Write the page.** Sections in the current order, with: the pieces table gaining `ui-self-hosted`; the "what it covers" table rows for forwarding window, failback, and labels updated from Task 2; a sentence that `TEMPORAL_ADDRESS` restarts stand in for the team's routing layer; the sidecar sentence from Global constraints; the ports sentence; Phase 3 split into fail over, forwarding window, flip, verify; Roll back as Task 2 Step 5. Every output shown is one Task 2 recorded.
- [ ] **Step 2: Verify render and style.**

Run: `"/Applications/Google Chrome.app/Contents/MacOS/Google Chrome" --headless --disable-gpu --dump-dom "file://$PWD/cloud-gateway/rehearsal/index.html" | grep -c '\$ '` and `grep -nP '[\x{2013}\x{2014}]' cloud-gateway/rehearsal/index.html`
Expected: a non-zero count; no dash matches.

- [ ] **Step 3: Commit** (ask Muto for the subject).

### Task 4: playbook page, interactive kit

**Files:**
- Create: `cloud-gateway/index.html`, copied from `index.html`.

**Interfaces:**
- Produces: element ids kept from the current page (`acct`, `nslist`, `keypat`, `keymap`, `cfg-proxy`, `cfg-s2s`, `tracker-body`, `next-up`, `handoff-summary`, `handoff-bar`, `phase4-status`, `stages`, `diagram-main`, `diagram-end`, `binding-track`), plus new section ids `questions`, `window`, `failback`, `monitoring`.
- Produces for Task 5: `data-check` keys on each checklist item, prefixed by section as now.

- [ ] **Step 1: State.** `STORE = "temporal-proxy:migrating-to-cloud:cloud-gateway:v1"`; `STEPS = ["cond", "failover", "flipped", "verified"]`; tracker column headers "1. Conditions", "2. Failed over", "3. Clients flipped", "4. Verified".
- [ ] **Step 2: `proxyYaml()`.** No rules, no anchor, no self-hosted upstream. `routing: { default: cloud, system: cloud-system }` with the `cloud` and `cloud-system` upstreams as in the current Phase 4 block (suffix rule on both, `${TEMPORAL_API_KEY}`). Encryption as now, binding following `strict()`. Comment above `cloud`: "Gets no traffic until the first Namespace's clients flip." until any `flipped`, then "Serves every Namespace whose clients have flipped." `s2sYaml()` unchanged.
- [ ] **Step 3: `warningsFor(n)`, exact copy:**
  - `flipped && !failover`: "Clients flipped before failover: they reach a passive Namespace in Cloud and get NamespaceNotActive."
  - `(failover || flipped) && !strict()`: "Strict isn't on yet; the plan switches before the first handoff."
  - `(failover || flipped) && !cond`: "Handoff conditions weren't checked."
  - `failover && !flipped`: "In the forwarding window: clients still on self-hosted depend on forwarding. Flip soon."
  - `verified && !flipped`: "Verified, but clients haven't flipped."
- [ ] **Step 4: `statusPill`, next-up, Phase 4 status.** "in Cloud" when `flipped && verified`; "handing off" when `failover || flipped`; else "self-hosted". Next up: not failed over gives "Next up is `ns`. Fail it over first, then flip its clients."; flipped but not failed over gives the warn callout "`ns` has clients flipped but hasn't failed over. Fail it over now, or flip its clients back."; failed over but not flipped gives "`ns` has failed over. Flip its clients to the gateway, then verify."; else "flip done, verify". `pinned()` becomes "Namespaces not yet in Cloud".
- [ ] **Step 5: Stages and diagram.** `STAGES` ids `init`, `p1`, `p2`, `p3`, `end`. Captions: p1 "Temporal Proxy is up in front of Cloud, sealing nothing yet. Apps still call the self-hosted Temporal Service directly, which stores plaintext."; p2 "s2s-proxy replicates each Namespace to Cloud, sealing everything under the shared K_repl and labelling it with the Cloud name. Cloud holds passive replicas."; p3 "Namespaces hand off one at a time: fail over, then flip the Namespace's clients to the gateway. Between the two, clients still on self-hosted are forwarded through s2s-proxy under the Namespace's key."; end as now. `drawDiagram` view: `active` from `failover`, `routed` from `flipped`; in `init`/`p1`/`p2` apps point at self-hosted and the proxy box has no inbound edge; a Namespace with `failover && !flipped` draws its edge to self-hosted plus a dashed forwarding edge through s2s-proxy.
- [ ] **Step 6: Verify interactive behavior headlessly.** Write a throwaway `.work/check.html` (ignored) that loads `cloud-gateway/index.html` in an iframe, seeds `localStorage` with `{namespaces:["ns1","ns2"], ns:{ns1:{flipped:true}}, checks:{}, account:"", keyPattern:""}`, and writes the tracker's warnings and `cfg-proxy` text into `document.body`. Serve the repo root with `python3 -m http.server 8999` so both pages share an origin (on `file://` they may not share `localStorage`), then run `--dump-dom http://localhost:8999/.work/check.html`.
Expected: the ns1 row contains "Clients flipped before failover" (review focus 1); `cfg-proxy` contains `system: cloud-system` and no `rules:`; with `namespaces:[]` and blank account, no JS error in `--enable-logging=stderr --v=0` output and `<account>` appears (review focus 3); seeding `ns1:{}` after a failback state shows "self-hosted" and no warnings (review focus 2).
- [ ] **Step 7: Commit** (ask Muto for the subject).

### Task 5: playbook page, prose

**Files:**
- Modify: `cloud-gateway/index.html` (content sections only)

**Interfaces:**
- Consumes: ids and `data-check` keys from Task 4; FACTS from Task 2.

- [ ] **Step 1: Write each section per the spec's "Playbook structure" and "The five questions" sections.** Keys table exactly as the spec's. Who holds what: Temporal Proxy wraps and unwraps K_ns and K_proxy, unwraps K_repl; s2s-proxy wraps and unwraps K_ns and K_repl, unwraps K_proxy. Phase 4 K_repl retirement: "every Workflow running at its Namespace's failover has closed, and the longest retention period has passed since, plus archives." The Monitoring section lists the customer's base counters verbatim, then the additions, each with what it means and when it must be zero. The questions section links each of the five to its answer. Confirm before starting gets the spec's five items, minus any Task 2 showed (those move to the rehearsal page's "covers" table), with the forwarding caveat kept.
- [ ] **Step 2: Verify style and links.**

Run: `grep -nP '[\x{2013}\x{2014}]' cloud-gateway/index.html; grep -o 'href="#[^"]*"' cloud-gateway/index.html | sort -u | sed 's/href="#//;s/"//' | while read id; do grep -q "id=\"$id\"" cloud-gateway/index.html || echo "missing #$id"; done`
Expected: no output from either.

- [ ] **Step 3: Read the page against the spec's anti-goals.** Note any sentence that only makes sense next to the current playbook, or that claims something not in Task 2's FACTS. Fix them.
- [ ] **Step 4: Commit** (ask Muto for the subject).

### Task 6: link from the current playbook

**Files:**
- Modify: `index.html`, one callout right after the header's draft notice.

- [ ] **Step 1: Add the callout.** Copy: "Apps keep talking to the self-hosted Temporal Service until their Namespace moves, and Temporal Proxy sits in front of Cloud only? Use the [cloud gateway playbook](cloud-gateway/) instead." Add the reverse link at the top of `cloud-gateway/index.html` if Task 5 didn't.
- [ ] **Step 2: Verify the diff is that callout only.**

Run: `git diff --stat index.html`
Expected: one file, a few lines added, none removed.

- [ ] **Step 3: Commit** with the spec and plan docs (ask Muto for the subject).

```bash
git add index.html docs/superpowers/specs/2026-10-29-cloud-gateway-playbook-design.md docs/superpowers/plans/2026-10-29-cloud-gateway-playbook.md
```
