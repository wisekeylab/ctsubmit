# ctsubmit: REST API Documentation

## OpenAPI

An [OpenAPI](https://swagger.io/specification/) definition for ctsubmit is provided by [openapi.yaml](/docs/openapi.yaml) and rendered in HTML [here](https://crtsh.github.io/ctsubmit/openapi.html).

## POST Endpoints

These endpoints accept a JSON request body containing a certificate chain and (extending the RFC6962 APIs) submission options. They return SCTs collected from CT logs.

| Endpoint | Description |
|---|---|
| `/add-chain` | Submit an X.509 certificate to CT logs. |
| `/add-pre-chain` | Submit a precertificate to CT logs. |

### Request Body

The request body is a JSON object with the following properties:

| Name | Required? | Default | Description |
|---|---|---|---|
| `chain` | Required | — | An array of base64-encoded DER certificates. The first element is the end-entity certificate; subsequent elements chain to the previous and so on to the root or a certificate that chains to a known root certificate. |
| `policyCompliant` | Optional | `true` | If true, ensure the resulting SCT list meets the requirements of the applicable CT policies. |
| `testLogs` | Optional | `false` | If true, submit to test CT logs instead of production CT logs. |
| `mimics` | Optional | `false` | If true, also generate SCTs from the [log mimics](https://googlechrome.github.io/CertificateTransparency/3p_libraries.html). |
| `operators` | Optional | `1` | The minimum number of distinct log operators from which to obtain SCTs. Overridden if `policyCompliant` is true and the applicable CT policies require more. |
| `scts` | Optional | `1` | The minimum number of SCTs to obtain. Overridden if `policyCompliant` is true and the applicable CT policies require more. |
| `requireAtLeastOneRFC6962SCT` | Optional | `false` | If true, require at least one SCT from an RFC 6962 log. Automatically set to true when `policyCompliant` is true (Apple CT policy). |
| `preferAtLeastOneStaticSCT` | Optional | `false` | If true, prefer at least one SCT from a Static CT API log. Automatically set to true when `policyCompliant` is true (Mozilla preference). |
| `verbose` | Optional | `false` | If true, include the submission strategy details in the response. |

### Response Format

The response format can be selected using the `format` query parameter or the `Accept` header:

| Format | Query parameter | Content-Type |
|---|---|---|
| JSON (default) | `?format=json` | `application/json` |
| HTML | `?format=html` | `text/html` |

### Example Request

```bash
curl -X POST https://ctsubm.it/add-chain \
  -H "Content-Type: application/json" \
  -d '{
    "chain": [
      "<base64-encoded leaf certificate>",
      "<base64-encoded intermediate certificate>"
    ],
    "policyCompliant": true
  }'
```

### Success Response (HTTP 200)

A successful response contains the collected SCTs and, for precertificate submissions, additional fields:

```json
{
  "logResponse": [
    {
      "sct_version": 0,
      "id": "<base64-encoded log ID>",
      "timestamp": 1234567890123,
      "extensions": "",
      "signature": "<base64-encoded signature>"
    }
  ],
  "finalTBSCertB64": "<base64-encoded TBSCertificate>",
  "ctlint": [
    {
      "finding": "Description of the finding",
      "severity": "warning"
    }
  ],
  "strategy": [...]
}
```

| Field | Presence | Description |
|---|---|---|
| `logResponse` | Always | Array of SCT responses from CT logs. |
| `finalTBSCertB64` | `add-pre-chain` only | Base64-encoded TBSCertificate with the collected SCTs embedded as an SCT list extension and the CT poison extension removed. Ready for signing. |
| `ctlint` | `add-pre-chain` only | Array of [ctlint](https://github.com/crtsh/ctlint) findings for CT policy compliance checking. |
| `strategy` | When `verbose=true` | Array of strategy members showing which logs were considered, their priority buckets, and submission outcomes. |

### Error Response (HTTP 400)

Error responses use [RFC 7807 Problem Details](https://www.rfc-editor.org/rfc/rfc7807):

```json
{
  "type": "about:blank",
  "title": "Bad Request",
  "detail": "Description of the error"
}
```

### Timeout Response (HTTP 503)

If the request times out (exceeds `server.requestTimeout`), a `503 Service Unavailable` response is returned.

## Policy-Compliant Submissions

When `policyCompliant` is `true` (the default), ctsubmit automatically enforces the SCT requirements from the applicable CT policies:

| Requirement | Value |
|---|---|
| Minimum SCTs | 2 (or 3 if certificate validity > 180 days) |
| Minimum distinct operators | 2 |
| Require at least one RFC 6962 SCT | Yes (Apple CT policy) |
| Prefer at least one Static CT SCT | Yes (Mozilla preference) |

For BIMI Mark Certificates, the Usable BIMI log list is used instead of the Usable TLS log list.

## GET Endpoints

### Web Forms

Browse (i.e., send a GET request) to the `add-chain` or `add-pre-chain` endpoint to access an interactive submission form where you can paste a PEM-encoded certificate chain and configure submission options.

### Log Lists

| Endpoint | Description |
|---|---|
| `/usable_tls_logs.json` | Usable TLS CT logs — the intersection of logs marked as usable by Chrome, Apple, and Mozilla. Used for policy-compliant TLS submissions. |
| `/active_tls_logs.json` | Active TLS CT logs — all non-test logs from the crt.sh active log list. Used for non-policy-compliant submissions. |
| `/test_tls_logs.json` | Test CT logs — test-flagged logs from the crt.sh active log list. Used when `testLogs` is true. |
| `/usable_bimi_logs.json` | Usable BIMI CT logs — BIMI-approved logs from the crt.sh approved log list. Used for BIMI Mark Certificate submissions. |

### Dashboard

| Endpoint | Description |
|---|---|
| `/dashboard` | Submission dashboard showing per-log monitoring data. |

The dashboard accepts an optional `loglist` query parameter:

| Value | Description |
|---|---|
| `usabletls` (default) | Usable TLS logs |
| `activetls` | Active TLS logs |
| `testtls` | Test logs |
| `usablebimi` | Usable BIMI logs |

The dashboard displays the following information for each log:

- **STH status** — tree size, Maximum Merge Delay, and STH age.
- **Endpoint uptime** — 24-hour and 90-day uptime percentages.
- **Recent outcomes** — submission success/failure counts in the last 30 seconds.
- **Response latency** — average response time in the last 30 seconds.
- **Backoff state** — current backoff/dispreferal status and reason.

## CTLint Severity Levels

For precertificate submissions, ctlint findings are included in the response with the following severity levels:

| Severity | Description |
|---|---|
| `info` | Informational finding. |
| `notice` | Notable observation. |
| `warning` | Potential issue that may indicate non-compliance. |
| `error` | Non-compliance with a CT policy requirement. |
| `bug` | Likely a bug in the linter itself. |
| `fatal` | Critical error that prevents further processing. |

## Strategy Buckets

When `verbose=true`, each log in the strategy response is assigned to a bucket indicating its priority:

| Priority | Bucket | Meaning |
|---|---|---|
| Highest | `PREFERRED_BYCONFIG` | Log URL matches a configured preference regex. |
| | `NEUTRAL` | Default — no special signals. |
| | `DISPREFERRED_SLOWRESPONSES` | Recent slow response backoff in effect. |
| | `DISPREFERRED_RECENT4XX` | Recent HTTP 4xx backoff in effect. |
| | `DISPREFERRED_RECENT5XX` | Recent HTTP 5xx backoff in effect. |
| | `DISPREFERRED_RECENTTIMEOUT` | Recent timeout backoff in effect. |
| | `DISPREFERRED_RECENTBADRESPONSE` | Recent bad response backoff in effect. |
| | `DISPREFERRED_LOWUPTIME` | Low submission-endpoint uptime. |
| | `DISPREFERRED_MMDBLOWN` | STH age exceeds the log's Maximum Merge Delay. |
| Lowest | `EXCLUDED` | Explicitly excluded by configuration, or no STH data available. |

For a detailed explanation of the submission strategy, see [SubmissionStrategy.md](/docs/SubmissionStrategy.md).
