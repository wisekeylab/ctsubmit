# Custom Static Test Logs: Required Changes

This document specifies the application changes required for ctsubmit to support additional WISeKey internal Static CT logs for testing.

The goal is to let a test deployment load internal Static CT logs and accepted roots from mounted configuration, without changing production behavior and without requiring those logs to be present in the upstream `ctloglists` module.

## Scope

In scope:

- Static CT logs only.
- Test deployments only.
- Loading additional logs from local configuration files.
- Submitting test requests to public test logs, internal static test logs, or both.
- Verifying returned SCTs from internal static test logs.
- Validating submitted chains against accepted roots configured for each internal static test log.
- Exposing internal static test logs in test log-list and dashboard views.

Out of scope:

- Adding production logs through local config.
- Replacing the upstream production log lists.
- RFC6962 internal logs.
- Changing browser CT policy logic.
- Using internal static logs for real production certificate issuance.

## Current Behavior

ctsubmit currently builds its log lists and accepted-root mappings from compiled Go module data:

- `github.com/crtsh/ctloglists`
- `github.com/crtsh/ccadb_data`

The request field `testLogs: true` makes ctsubmit use the test TLS log list. However, that list is derived from the compiled crt.sh active log list. A log that is not present there cannot be selected only through `config.yaml`.

Important current code paths:

| Area | Current source |
|---|---|
| Test log list creation | `loglists/filter.go` |
| Chain validation | `pki/chain.go` |
| SCT signature verification | `submitter/submitToLog.go` |
| Static checkpoint monitoring | `monitor/sthMonitor.go` |
| Backoff tracking | `monitor/backoffHandler.go` |
| Uptime tracking | `monitor/uptimeFetcher.go` |
| Strategy construction | `submitter/strategy.go` |

## Proposed Configuration

Add a test-only configuration section:

```yaml
customStaticTestLogs:
  enabled: true
  includeWithPublicTestLogs: true
  logs:
    - operator: "WISeKey Internal"
      name: "WISeKey Static Test Log 1"
      submissionURL: "https://ctlog-test-1.example.com/"
      monitoringURL: "https://ctlog-test-1.example.com/"
      checkpointOrigin: "ctlog-test-1.example.com"
      mmd: 86400
      publicKeyFile: "/config/static-logs/ctlog-test-1-public-key.pem"
      acceptedRootsDir: "/config/static-logs/ctlog-test-1-roots/"
```

Field meanings:

| Field | Required | Description |
|---|---|---|
| `enabled` | Yes | Enables loading custom static test logs. Default should be `false`. |
| `includeWithPublicTestLogs` | No | If `true`, append internal logs to the normal public test log list. Default should be `true` for operational simplicity. |
| `operator` | Yes | Operator name used for quorum diversity and dashboard display. |
| `name` | Yes | Human-readable log name. |
| `submissionURL` | Yes | Static CT submission base URL. |
| `monitoringURL` | Yes | Static CT monitoring base URL used to fetch checkpoints. |
| `checkpointOrigin` | Yes | Expected Static CT checkpoint origin/key name. This must match the checkpoint note origin. |
| `mmd` | Yes | Maximum Merge Delay in seconds. |
| `publicKeyFile` | Yes | PEM file containing the static log public key. |
| `acceptedRootsDir` | Yes | Directory containing PEM or DER accepted root certificates for this log. |

## Request Behavior

Existing test requests should continue to work:

```json
{
  "chain": [
    "<base64 test leaf or precertificate>",
    "<base64 test intermediate>"
  ],
  "testLogs": true,
  "policyCompliant": false,
  "operators": 1,
  "scts": 1
}
```

If `customStaticTestLogs.enabled` is true and `includeWithPublicTestLogs` is true, `testLogs: true` should consider both:

- public test CT logs from the compiled test log list
- custom WISeKey internal Static CT logs from config

A later enhancement may add a request field to choose the log source explicitly, for example:

```json
{
  "testLogs": true,
  "testLogSource": "public|internal|all"
}
```

That selector is not required for the first implementation if internal testing runs as a separate service.

## Implementation Requirements

### 1. Configuration Model

Extend `config.Config` with `CustomStaticTestLogs`.

Defaults:

```text
customStaticTestLogs.enabled = false
customStaticTestLogs.includeWithPublicTestLogs = true
customStaticTestLogs.logs = []
```

Validation should reject:

- enabled configuration with no logs
- missing `operator`, `name`, `submissionURL`, `monitoringURL`, `checkpointOrigin`, `publicKeyFile`, or `acceptedRootsDir`
- non-positive `mmd`
- unparsable public keys
- accepted-root directories with no usable certificates
- duplicate log IDs
- malformed URLs

### 2. Custom Log Registry

Add a package-level registry for custom static test logs. The registry should provide:

- the configured logs as `loglist3.TiledLog` entries
- accepted-root pools by log ID
- signature verifiers by log ID
- checkpoint verifier metadata by monitoring URL
- log name lookup metadata

This keeps custom log behavior explicit and avoids mutating upstream module globals directly.

### 3. Log ID and Public Key Handling

For each configured log:

1. Read `publicKeyFile`.
2. Parse the PEM public key.
3. Create the Static CT/RFC6962 verifier.
4. Compute the log ID using the same scheme used by CT log lists for Static CT logs.
5. Store the verifier by log ID.
6. Use the log ID in the `TiledLog` entry.

The implementation must verify that SCT signature validation uses the custom verifier when the SCT log ID belongs to a configured internal static log.

### 4. Accepted Roots

For each configured log:

1. Load certificates from `acceptedRootsDir`.
2. Accept PEM and DER files.
3. Build an accepted-root pool for that log.
4. Register the pool by log ID in the custom registry.
5. Make chain validation consult the custom registry when the log ID is not found in `ctloglists.LogAcceptedRootsMap`.

The submitted chain must still validate to a root accepted by the selected log. Custom logs must not bypass chain validation.

### 5. Test Log List Construction

When custom static test logs are enabled:

1. Build the normal public `TestTLSLogs` as today.
2. Append configured logs as `TiledLog` entries.
3. Preserve operator grouping:
   - logs with the same configured operator should be under the same operator entry
   - logs from different operators must remain distinct for quorum checks
4. Mark these logs as test-only.

The `/test_tls_logs.json` endpoint and `dashboard?loglist=testtls` should show the configured static test logs.

### 6. SCT Signature Verification

Update SCT verification to check both:

1. upstream `ctloglists.LogSignatureVerifierMap`
2. custom static test log verifiers

If neither contains the SCT log ID, keep the current behavior and reject the SCT as unknown.

### 7. Checkpoint Monitoring

Static CT log strategy relies on checkpoint freshness. Custom static test logs must be registered with monitoring so they are not excluded as missing STH/checkpoint data.

For each configured log, monitoring needs:

- `submissionURL`
- `monitoringURL`
- `checkpointOrigin`
- note verifier
- MMD

Important: the current static checkpoint monitor derives the checkpoint origin/key name from `SubmissionURL`. For internal static logs this must become configurable through `checkpointOrigin`.

### 8. Backoff and Metrics

Custom static test logs must be registered in the same runtime maps used for:

- bad response backoff
- timeout backoff
- HTTP 4xx backoff
- HTTP 5xx backoff
- slow response backoff
- response-time metrics
- submission outcome metrics

This prevents internal logs from being selected without normal reliability controls.

### 9. Uptime Handling

Google's public CT uptime feeds will not include WISeKey internal logs.

For custom static test logs, choose one of these behaviors:

- preferred for first implementation: treat missing external uptime data as neutral for custom logs
- alternative: allow configured static uptime values or disable uptime checks for custom logs

Do not mark custom internal logs as excluded only because they are absent from Google's public uptime CSV.

### 10. Safety Controls

The feature must be disabled by default.

Recommended safety controls:

- only load custom static logs when `customStaticTestLogs.enabled` is true
- log each loaded custom static test log at startup
- fail startup if enabled config is invalid
- expose custom static logs only through test log-list behavior
- document that production deployments must not mount internal test-log config

## Testing Plan

Unit tests:

- config parsing accepts a valid custom static test log
- config parsing rejects missing required fields
- public key parsing and log ID derivation are stable
- accepted roots are loaded from PEM and DER files
- duplicate log IDs are rejected
- custom logs are appended to `TestTLSLogs`
- production usable log lists are unchanged when custom test logs are enabled
- chain validation succeeds for a chain rooted in a configured accepted root
- chain validation fails for an unaccepted root
- SCT signature verification succeeds for a custom static test log SCT
- unknown SCT log IDs remain rejected

Integration tests:

- `/test_tls_logs.json` includes configured internal static logs
- `/dashboard?loglist=testtls` includes configured internal static logs
- a `testLogs: true` request can obtain an SCT from an internal static test log
- `policyCompliant: false`, `operators: 1`, `scts: 1` works with a single internal operator
- backoff is applied after timeout or bad response
- checkpoint monitoring accepts a valid checkpoint with the configured `checkpointOrigin`
- checkpoint monitoring rejects a checkpoint with the wrong origin

Operational validation:

```bash
curl -fsS https://<internal-test-ctsubmit-host>/test_tls_logs.json
curl -fsS https://<internal-test-ctsubmit-host>/dashboard?loglist=testtls
```

Submit a test chain with:

```json
{
  "chain": [
    "<base64 test leaf or precertificate>",
    "<base64 test intermediate>"
  ],
  "testLogs": true,
  "policyCompliant": false,
  "operators": 1,
  "scts": 1,
  "verbose": true
}
```

Confirm that the response strategy includes the internal Static CT log and that the returned SCT verifies.

## Rollout Plan

1. Implement the feature on `all-improvements`.
2. Add unit and integration tests.
3. Build a test image with a commit-specific tag.
4. Deploy only to `ctsubmit-internal-test.<domain>`.
5. Mount internal static log config and accepted roots into `/config`.
6. Validate `/test_tls_logs.json`, dashboard, checkpoint monitoring, and test submissions.
7. Keep production deployments on the same image only if `customStaticTestLogs.enabled` remains unset or false.

## Branch and Pull Request Strategy

This change is logically independent from the current WISeKey improvements, but it is not completely file-independent.

Expected overlap with existing improvements:

| File | Reason |
|---|---|
| `pki/chain.go` | Custom accepted-root lookup must be added to the same chain-validation path that was improved by the chain-validation cache fix. |
| `submitter/submitToLog.go` | Custom SCT verifier lookup belongs near existing SCT signature verification. This is a low-conflict area because the current improvement in this file is request-cancellation handling. |

Expected independent files:

| File area | Reason |
|---|---|
| `config/config.go` | New configuration fields and defaults. |
| `loglists/filter.go` | Append custom static logs to the test TLS log list. |
| `monitor/*.go` | Register custom static logs for checkpoint monitoring, backoff, uptime handling, and metrics. |
| docs/tests | New documentation and focused tests. |

Recommended approach for WISeKey deployment:

1. Create the implementation branch from `all-improvements`.
2. Keep the commits focused on the custom Static CT test-log feature.
3. Rebase onto updated `all-improvements` as upstream changes arrive.
4. Deploy only to the internal test service first.

Recommended approach for an upstream pull request:

1. Prepare a separate PR branch from `main` or `upstream/main`.
2. Keep the feature generic, disabled by default, and not WISeKey-branded in code.
3. Expect a small conflict or manual adaptation around `pki/chain.go` if the PR is later combined with `all-improvements`.
4. Do not make the upstream PR depend on Docker or request-cancellation improvements unless maintainers ask for that.

In short: it can be an independent pull request, but for WISeKey's own working branch it is cleaner to implement it on top of `all-improvements` because that is the branch we intend to deploy.

## Open Decisions

- Whether the first implementation should append custom logs to all `testLogs: true` requests or add an explicit request selector.
- Whether accepted roots should be loaded from one directory per log or from named root bundles shared across logs.
- Whether internal static logs need custom uptime configuration or should simply ignore missing public uptime data.
- Whether internal static test logs should be exposed in a separate endpoint in addition to `/test_tls_logs.json`.
