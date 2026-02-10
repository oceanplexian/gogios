# NRDP Relay API Specification (as implemented by nrdc)

Dense reference for LLM agents implementing an NRDP-compatible receiver/sender.

## Endpoint

```
POST /relay
```

Only POST is accepted (405 on anything else). Additional endpoints: `/debug/vars` (expvar metrics, GET).

## Authentication

Token-based via bcrypt (cost 14). Token transmitted as form field or query param `token`. Server stores `token_hash` (bcrypt hash). Requests from `127.0.0.1` bypass auth. Empty token or empty hash = reject. On auth failure: `401 Unauthorized`, body `"authorization failed\n"`.

## Content Negotiation (4 accepted formats)

| Format | Content-Type | Payload Location |
|--------|-------------|-----------------|
| XML form (native NRDP) | `application/x-www-form-urlencoded` | form field `XMLDATA` |
| JSON form | `application/x-www-form-urlencoded` | form field `JSONDATA` |
| Raw XML | `text/xml` or `application/xml` | request body |
| Raw JSON | `application/json` | request body |

Response Content-Type mirrors request Content-Type. Falls back to `text/plain` on marshal failure.

## Request Payloads

### XML

```xml
<?xml version="1.0" encoding="utf-8"?>
<checkresults>
  <checkresult type="service" checktype="1">
    <hostname>web01.example.com</hostname>
    <servicename>HTTP</servicename>
    <state>0</state>
    <output>OK - 200 response in 0.5s | time=0.5s</output>
    <timestamp>2024-02-10T12:34:56Z</timestamp>
  </checkresult>
</checkresults>
```

### JSON

```json
{
  "checkresults": [
    {
      "type": "service",
      "hostname": "web01.example.com",
      "servicename": "HTTP",
      "status": 0,
      "output": "OK - 200 response in 0.5s | time=0.5s",
      "timestamp": "2024-02-10T12:34:56Z"
    }
  ]
}
```

### Field Reference

| Field | XML tag | JSON key | Type | Required | Notes |
|-------|---------|----------|------|----------|-------|
| Type | `type` (attr) | `type` | string | yes | Always `"service"` |
| Hostname | `<hostname>` | `hostname` | string | yes | FQDN of monitored host |
| Service Name | `<servicename>` | `servicename` | string | yes | Check/service identifier |
| Status | `<state>` | `status` | int | yes | 0=OK, 1=WARNING, 2=CRITICAL, 3=UNKNOWN. Values >3 clamped to 3 |
| Output | `<output>` | `output` | string | yes | Plugin output. Optional perfdata after `\|` |
| Timestamp | `<timestamp>` | `timestamp` | string | no | Any parseable format (ISO 8601, RFC 3339, epoch, etc via `dateparse`). Defaults to `time.Now()` if empty/unparseable |
| Checktype | `checktype` (attr) | — | string | no | Always `"1"` (passive). Informational only |

### Status Codes (Nagios-compatible)

| Code | Name | Meaning |
|------|------|---------|
| 0 | OK / Normal | Healthy |
| 1 | WARNING | Warning condition |
| 2 | CRITICAL | Critical failure |
| 3 | UNKNOWN | Indeterminate |
| 4 | PENDING | Never checked (internal only, not sent over wire) |

## Response

### Format

Response format matches request format.

**XML response:**
```xml
<?xml version="1.0" encoding="UTF-8"?>
<response>
  <id>XYZ</id>
  <status>200</status>
  <message>Processing 2 Results</message>
</response>
```

**JSON response:**
```json
{"id":"XYZ","status":200,"message":"Processing 2 Results"}
```

**Plain text fallback:**
```
Processing 2 Results
```

### Response Fields

- `id`: 3 random uppercase ASCII letters (A-Z), generated per request, used for log correlation
- `status`: HTTP status code (200, 401, 405, 500)
- `message`: `"Processing {N} Results"` on success, error description on failure

### HTTP Status Codes

| Code | Condition |
|------|-----------|
| 200 | Results accepted |
| 401 | Invalid/missing token (non-localhost) |
| 405 | Non-POST method |
| 500 | Payload decode failure |

## Sending Results (Outbound/Receiver Config)

When nrdc sends results to a downstream NRDP receiver, it uses Go templates and HTTP POST.

### Native NRDP form POST (standard Nagios NRDP)

```
POST /nrdp/ HTTP/1.1
Content-Type: application/x-www-form-urlencoded

XMLDATA=<url-encoded-xml>&token=SECRET&cmd=submitcheck
```

The `cmd=submitcheck` param and `token` param are passed via `http_vars` config. The XML payload is URL-encoded into the form field named by `http_data_var` (default `XMLDATA`).

### NRDP XML Template (built-in)

```xml
<?xml version="1.0" encoding="utf-8"?>
<checkresults>
{{- range . }}
  <checkresult type="service" checktype="1">
    <hostname>{{.Hostname}}</hostname>
    <servicename>{{.Servicename}}</servicename>
    <state>{{.Status}}</state>
    <output>{{.Msg}}</output>
  </checkresult>
{{ end -}}
</checkresults>
```

### JSON Template (built-in)

```json
{"checkresults": [
{{- range $index, $servicecheck := . }}
  {{if $index}}},{{end}}{
    "type": "service",
    "hostname": "{{$servicecheck.Hostname}}",
    "servicename": "{{$servicecheck.Servicename}}",
    "state": {{$servicecheck.Status}},
    "timestamp": "{{$servicecheck.Ts}}",
    "output": {{$servicecheck.Msg}}
{{- end}}
  }
]}
```

### Template Context Fields

| Field | Type | Description |
|-------|------|-------------|
| `.Hostname` | string | Host FQDN |
| `.Servicename` | string | Service/check name |
| `.Status` | int | 0-3 |
| `.Statusname` | string | "OK", "WARNING", "CRITICAL", "UNKNOWN" |
| `.Msg` | string | Escaped output text (JSON-safe when used in JSON templates) |
| `.Ts` | string | Unix epoch timestamp |
| `.TmplData` | string | Custom per-receiver template data from config |

## Output & Performance Data

Output format follows Nagios plugin conventions: `human-readable text | key=value key2=value2`. The pipe `|` separates display output from structured perfdata. Control characters (except newline U+000A) are stripped. Newlines in output are replaced with the configured `replace_newlines` string (default `\n` literal).

## Source Tracking

Each incoming result is tagged with a source string: `{protocol}://{remote_ip}:{port}` where protocol is one of `xml`, `json`, `xmlform`, `jsonform`.

## Timing & Retry

| Parameter | Default | Range | Description |
|-----------|---------|-------|-------------|
| `interval` | 275s | 15s–24h | How often to push results to receiver |
| `retry_interval` | same as interval | 15s–24h | Interval after failed push |
| `initial_delay` | 2m | 1s–1h | Delay before first push after startup |
| `timeout` | 9s | — | HTTP client timeout per request |

On success: next update scheduled at `interval`. On failure (HTTP error, unexpected status code, timeout): next update at `retry_interval`, `failed_updates` counter incremented. HTTP redirects are NOT followed (`http.ErrUseLastResponse`).

## TLS

Server: optional, configured via `ssl_cert` + `ssl_key`. Client: `allow_invalid_ssl = true` disables certificate verification.

## Receiver Config Example (TOML)

```toml
[[receivers]]
url = "https://monitor.example.com/nrdp/"
method = "POST"
content_type = "application/x-www-form-urlencoded"
http_data_var = "XMLDATA"
http_vars = "token=mytoken123&cmd=submitcheck"
template_file = "/etc/nrdc/nrdp.template.xml"
interval = "5m"
retry_interval = "5m"
initial_delay = "2m"
timeout = "9s"
expected_code = 200
allow_invalid_ssl = false
```

## Minimal Drop-in Replacement Requirements

To replace Nagios and keep nrdc happy, your server must:

1. Accept `POST /relay` (or whatever path nrdc is configured to send to)
2. Accept `application/x-www-form-urlencoded` with `XMLDATA` form field containing the XML payload above, plus `token` and `cmd=submitcheck` as additional form fields
3. Validate the `token` field (or accept all tokens)
4. Parse the `<checkresults>` XML: extract `hostname`, `servicename`, `state` (int 0-3), `output` (string), optional `timestamp`
5. Return HTTP 200 on success (nrdc checks `expected_code`)
6. Optionally return an XML/JSON response body (nrdc logs it but doesn't parse it critically)
7. Handle multiple `<checkresult>` elements per request (batch)
8. Accept results periodically (default every ~5 minutes)

If you also want to **send** results to nrdc's relay endpoint, POST JSON or XML to `/relay` with the structures above and an optional `token` query param.
