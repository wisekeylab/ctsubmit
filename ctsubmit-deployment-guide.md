# ctsubmit Deployment Guide

This guide describes how to deploy either the original `ctsubmit` project or the WISeKey improved version from our fork, while keeping our fork aligned with the original upstream repository.

## Repository Model

Use the fork with two long-lived branch roles:

| Branch | Purpose |
|---|---|
| `main` | Clean mirror of the original project. Keep this aligned with `upstream/main`. Do not add WISeKey-only changes here. |
| `all-improvements` | WISeKey deployment branch. This branch is based on `main` and contains our improvements. |

Current remotes:

```bash
origin    https://github.com/wisekeylab/ctsubmit.git
upstream  https://github.com/crtsh/ctsubmit.git
```

Recommended branch flow:

```text
upstream/main
   |
   v
origin/main
   |
   v
origin/all-improvements
```

## Improvements Branch Contents

The `all-improvements` branch currently carries these WISeKey improvements on top of `main`:

| Commit | Change |
|---|---|
| `daa2a28` | Protect chain validation cache |
| `f56fa46` | Honor Docker `gomodfile` build argument |
| `7683c2d` | Propagate request cancellation to submissions |

The individual fix branches should remain available as reference branches:

```text
fix-chain-validation-cache
fix-dev-docker-gomodfile
fix-request-timeout-cancellation
```

## Keeping the Fork Current

Run this maintenance flow regularly, and before rebuilding production images:

```bash
git fetch --all --prune

git switch main
git merge --ff-only upstream/main
git push origin main

git switch all-improvements
git rebase main
```

After the rebase, check which WISeKey patches are still unique:

```bash
git cherry -v main all-improvements
```

Interpretation:

| Prefix | Meaning |
|---|---|
| `+` | Patch is still unique to `all-improvements`. |
| `-` | Patch is already present in `main` as an equivalent upstream change. |

If upstream accepts one of our pull requests, the rebase may skip that patch automatically, or it may produce a conflict if upstream implemented the same idea differently. Resolve conflicts case by case, run tests, then update the remote branch:

```bash
git push --force-with-lease origin all-improvements
```

Use `--force-with-lease`, not plain `--force`, so Git refuses to overwrite someone else's newer remote work.

## Build Inputs

The Docker image runs a statically built `ctsubmit` binary under the distroless non-root base image.

If publishing to GHCR, authenticate before pushing:

```bash
docker login ghcr.io
```

Default runtime behavior:

| Item | Value |
|---|---|
| Web/API port | `8080` |
| Monitoring port | `8081` |
| Config volume | `/config` |
| Config file path | `/config/config.yaml` |
| Entrypoint | `/sbin/tini -- /app/ctsubmit` |

The application also accepts configuration through environment variables. For example:

```bash
CTSUBMIT_SERVER_WEBSERVERPORT=8080
CTSUBMIT_SERVER_MONITORINGPORT=8081
```

## Deployment 1: Original Project Image

Use this deployment when DevOps wants an image that matches the original project without WISeKey-specific code changes.

This can be built from our fork's clean `main` branch, which should mirror `upstream/main`. To build directly from the original repository instead, use `https://github.com/crtsh/ctsubmit.git` as the clone URL.

### Checkout

```bash
git clone https://github.com/wisekeylab/ctsubmit.git
cd ctsubmit
git remote add upstream https://github.com/crtsh/ctsubmit.git
git fetch --all --prune
git switch main
git merge --ff-only upstream/main
```

### Build

Build with the original `go.mod` dependency set:

```bash
docker build \
  -t ghcr.io/wisekeylab/ctsubmit:original-main \
  .
```

For a reproducible tag, prefer the Git commit SHA:

```bash
export CTSUBMIT_REF="$(git rev-parse --short=12 HEAD)"

docker build \
  -t ghcr.io/wisekeylab/ctsubmit:original-${CTSUBMIT_REF} \
  .
```

Note: the original project's current `Dockerfile` does not support the `gomodfile` build argument. It always builds with `go.mod`.

### Run

```bash
docker run --rm \
  -p 8080:8080 \
  -p 8081:8081 \
  -v "$(pwd)/config.yaml:/config/config.yaml:ro" \
  ghcr.io/wisekeylab/ctsubmit:original-main
```

If no config file is required, omit the volume mount:

```bash
docker run --rm \
  -p 8080:8080 \
  -p 8081:8081 \
  ghcr.io/wisekeylab/ctsubmit:original-main
```

### Publish

```bash
docker push ghcr.io/wisekeylab/ctsubmit:original-main
```

For production, publish and deploy a commit-specific tag rather than a moving tag:

```bash
docker push ghcr.io/wisekeylab/ctsubmit:original-${CTSUBMIT_REF}
```

## Deployment 2: WISeKey Improved Image

Use this deployment for our operational image. It includes the WISeKey improvements and should be the normal target for internal testing and production evaluation.

### Checkout

```bash
git clone https://github.com/wisekeylab/ctsubmit.git
cd ctsubmit
git fetch --all --prune
git switch all-improvements
git rebase origin/main
```

If the local branch does not exist yet:

```bash
git switch -c all-improvements origin/all-improvements
git rebase origin/main
```

### Build With Standard Dependencies

This builds the improved code using `go.mod`:

```bash
docker build \
  -t ghcr.io/wisekeylab/ctsubmit:wisekey-improved \
  .
```

For a reproducible tag:

```bash
export CTSUBMIT_REF="$(git rev-parse --short=12 HEAD)"

docker build \
  -t ghcr.io/wisekeylab/ctsubmit:wisekey-${CTSUBMIT_REF} \
  .
```

### Build With Development Dependencies

The WISeKey improved Dockerfile supports selecting an alternate Go module file through `gomodfile`.

Use this when we intentionally want the dependency set from `dev_go.mod`:

```bash
docker build \
  --build-arg gomodfile=dev_go.mod \
  -t ghcr.io/wisekeylab/ctsubmit:wisekey-improved-devdeps \
  .
```

For a reproducible tag:

```bash
export CTSUBMIT_REF="$(git rev-parse --short=12 HEAD)"

docker build \
  --build-arg gomodfile=dev_go.mod \
  -t ghcr.io/wisekeylab/ctsubmit:wisekey-${CTSUBMIT_REF}-devdeps \
  .
```

### Run

```bash
docker run --rm \
  -p 8080:8080 \
  -p 8081:8081 \
  -v "$(pwd)/config.yaml:/config/config.yaml:ro" \
  ghcr.io/wisekeylab/ctsubmit:wisekey-improved
```

If no config file is required:

```bash
docker run --rm \
  -p 8080:8080 \
  -p 8081:8081 \
  ghcr.io/wisekeylab/ctsubmit:wisekey-improved
```

### Publish

```bash
docker push ghcr.io/wisekeylab/ctsubmit:wisekey-improved
docker push ghcr.io/wisekeylab/ctsubmit:wisekey-${CTSUBMIT_REF}
```

For the `dev_go.mod` dependency build:

```bash
docker push ghcr.io/wisekeylab/ctsubmit:wisekey-improved-devdeps
docker push ghcr.io/wisekeylab/ctsubmit:wisekey-${CTSUBMIT_REF}-devdeps
```

## Verification

After building either image, run a basic container smoke test:

```bash
export IMAGE=ghcr.io/wisekeylab/ctsubmit:wisekey-improved

docker run --rm \
  -p 8080:8080 \
  -p 8081:8081 \
  "${IMAGE}"
```

In another terminal:

```bash
curl -fsS http://localhost:8081/debug/build
curl -fsS http://localhost:8081/debug/config
curl -fsS http://localhost:8081/metrics
```

For Kubernetes, use the monitoring server for probes:

| Probe | Endpoint | Port |
|---|---|---|
| Liveness | `/livez` | `8081` |
| Readiness | `/readyz` | `8081` |
| Metrics | `/metrics` | `8081` |

## Production Recommendations

- Deploy commit-specific tags for production rollouts.
- Keep moving tags such as `wisekey-improved` only as convenience aliases.
- Record the Git branch and commit SHA in each deployment change ticket.
- Rebase `all-improvements` onto `main` regularly so upstream changes are incorporated early.
- Run tests after every rebase and before publishing a production image.
- Check `git cherry -v main all-improvements` after upstream accepts any of our PRs.
- Keep `main` clean so it can always be fast-forwarded from `upstream/main`.

## Test and Production Service Strategy

Run separate `ctsubmit` services for production and testing. They may use the same container image, but they should have separate DNS names, configuration, monitoring, deployment pipelines, and access controls.

| Service | Purpose | Log set | Certificate roots |
|---|---|---|---|
| Production | Real certificate issuance | Production usable CT logs | Publicly trusted production roots that are accepted by enough CT logs to meet quorum |
| Public-log test | Integration testing against external test CT logs | Public test CT logs from the crt.sh active log list | WISeKey test roots that are accepted by the selected test CT logs |
| Internal-log test | Integration testing against WISeKey-operated CT logs | WISeKey internal Static CT logs | WISeKey internal/test roots accepted by those internal logs |

### Production Service

Production clients should submit with the default production behavior:

```json
{
  "chain": [
    "<base64 leaf or precertificate>",
    "<base64 intermediate>"
  ],
  "policyCompliant": true,
  "testLogs": false
}
```

With `policyCompliant: true`, ctsubmit uses the usable production TLS log list and enforces CT policy requirements. For normal TLS certificates this means at least two distinct log operators and at least two SCTs, or three SCTs when the certificate validity is greater than 180 days.

Production requirements:

- Use only production roots and intermediates.
- Keep production and test endpoints separate.
- Do not allow test clients to submit to the production service.
- Monitor `/readyz`, `/livez`, `/metrics`, and the usable TLS dashboard.
- Use `verbose=true` only for troubleshooting, because it exposes log selection details in the response.

Useful production dashboard:

```text
https://<production-ctsubmit-host>/dashboard?loglist=usabletls
```

### Public-Log Test Service

Use this service when testing with WISeKey test roots that are accepted by external test CT logs, such as Sectigo and Google test logs.

Clients must set `testLogs: true` in each request:

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

Recommended starting point for tests is `policyCompliant: false`, with explicit `operators` and `scts` values that match the number of compatible test logs available for the test root. This avoids requiring browser CT policy quorum in an environment where only a few test logs may accept the test root.

When the test roots are accepted by enough independent test operators, DevOps or QA can also test stricter behavior:

```json
{
  "chain": [
    "<base64 test leaf or precertificate>",
    "<base64 test intermediate>"
  ],
  "testLogs": true,
  "policyCompliant": true,
  "verbose": true
}
```

Important behavior:

- `testLogs: true` switches ctsubmit to the test CT log list.
- `policyCompliant: true` still enforces SCT/operator quorum requirements, even though the request uses test logs.
- `policyCompliant: false` allows QA to request a smaller test quorum, for example one SCT from one operator.
- The submitted chain still must validate to a root accepted by the selected CT logs.

Useful public-log test endpoints:

```text
https://<test-ctsubmit-host>/test_tls_logs.json
https://<test-ctsubmit-host>/dashboard?loglist=testtls
```

To bias test submissions toward known compatible operators while keeping fallback logs available, configure preferred URL regexes:

```yaml
strategy:
  preferred:
    logURLRegex:
      - "sectigo"
      - "google"
```

If a problematic test log should never be used, exclude it by URL regex:

```yaml
strategy:
  excluded:
    logURLRegex:
      - "https://example-test-log.invalid/"
```

These settings only rank or exclude logs that ctsubmit already knows about. They do not add new logs.

### Internal Static-Log Test Service

Internal Static CT logs should be served from a dedicated test deployment, separate from both production and the public-log test service.

This deployment requires support for loading WISeKey internal Static CT logs and their accepted roots from test-service configuration. Until that support is implemented, ctsubmit can only submit to logs already present in its compiled log-list data. `strategy.preferred.*` and `strategy.excluded.*` can rank or exclude known logs, but they cannot add new internal logs.

Operational recommendation:

1. Use the public-log test service immediately for tests against Sectigo and Google test logs.
2. Add internal Static CT log support before routing requests to WISeKey internal logs.
3. Deploy internal-log testing as a separate service, for example `ctsubmit-internal-test.<domain>`.
4. Keep internal-log configuration out of production deployments.

The required application change is specified separately in [ctsubmit-custom-static-test-logs.md](ctsubmit-custom-static-test-logs.md).

### Mimic SCTs

For local client testing, requests may set `mimics: true` to generate SCTs from CT log mimics:

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
  "mimics": true
}
```

Mimic SCTs are useful for testing certificate assembly and SCT-list handling, but they are not a substitute for submitting to real external or internal CT logs. They do not prove that a real CT log accepted the chain.

### Test Service Recommendation

Start with one WISeKey improved test deployment:

```text
ctsubmit-test.<domain>
```

Use it for public test CT logs with `testLogs: true`, and require clients to explicitly choose test behavior in the request. Once internal CT log support is added, either:

- keep one test service and add a request/config option for the internal test log list, or
- run a second service dedicated to internal logs, for example `ctsubmit-internal-test.<domain>`.

The second option is operationally safer because it keeps public test-log behavior and internal-log behavior separate.

## Quick Reference

Update fork and improved branch:

```bash
git fetch --all --prune
git switch main
git merge --ff-only upstream/main
git push origin main
git switch all-improvements
git rebase main
git push --force-with-lease origin all-improvements
```

Build original:

```bash
git switch main
docker build -t ghcr.io/wisekeylab/ctsubmit:original-main .
```

Build improved:

```bash
git switch all-improvements
docker build -t ghcr.io/wisekeylab/ctsubmit:wisekey-improved .
```

Build improved with `dev_go.mod`:

```bash
git switch all-improvements
docker build --build-arg gomodfile=dev_go.mod -t ghcr.io/wisekeylab/ctsubmit:wisekey-improved-devdeps .
```
