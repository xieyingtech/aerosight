# DJI FlightHub 2 OpenAPI V2 contract (China public cloud)

This directory defines the sanitized upstream contract used by the AeroSight
`dji.flighthub2@1.0.0` connector. It intentionally contains no usable token,
organization name, project name, organization UUID, project UUID, or complete
device serial number.

## Deployment target

- Region: China mainland public cloud.
- Fixed API origin: `https://es-flight-api-cn.djigate.com`.
- Transport: HTTPS only. The origin is deployment-owned configuration and is
  never accepted from a browser request or connector credential.
- Contract checked: 2026-08-30.
- An unauthenticated request to
  `GET /openapi/v2.0/system_status` returned HTTP 200 with business error
  `{"code":200401,"message":"X-User-Token not found or empty"}`. This confirms
  the public-cloud gateway and also confirms that HTTP 200 does not imply
  business success.

## Credential and authorization

`X-User-Token` is the organization-level OpenAPI key (a JWT ticket). An
organization administrator obtains it in FlightHub 2 at:

`My Organization -> Organization Settings -> OpenAPI -> Copy Key`

The key is not an OAuth authorization code and FlightHub 2 does not provide a
redirect/callback authorization flow for this integration.

Every request sends:

- `X-User-Token: <redacted>`
- `X-Request-Id: <new UUID per attempt>`
- `X-Language: zh`

Project-scoped requests additionally send:

- `X-Project-Uuid: 00000000-0000-0000-0000-000000000000`

Tokens MUST NOT appear in URLs, query strings, response payloads, logs, audit
records, metrics, fixtures, or source control.

## Project discovery

`GET /openapi/v2.0/project`

Project discovery sends `X-User-Token` but does not send
`X-Project-Uuid`. AeroSight uses the following query contract:

| Parameter | Value | Notes |
| --- | --- | --- |
| `usage` | `complete` | Enables paged results. Official docs identify `simple` as the non-paged mode. |
| `sort_column` | `create_time` | Stable discovery ordering used by the official example. |
| `sort_type` | `desc` | Descending order. |
| `page` | 1-based integer | Official default is 1. |
| `page_size` | 20 | The published schema/example uses 20, despite one description line saying 10. AeroSight always sends 20 explicitly. |

Success is HTTP 200 and business `code: 0`. The response data required by the
connector is:

```json
{
  "code": 0,
  "message": "",
  "data": {
    "list": [
      {
        "name": "Sanitized project",
        "introduction": "",
        "uuid": "00000000-0000-0000-0000-000000000000",
        "org_uuid": "00000000-0000-0000-0000-000000000000",
        "created_at": 0,
        "updated_at": 0
      }
    ]
  }
}
```

The published response has no total/page cursor. AeroSight increments `page`
until `data.list.length < page_size`; it also rejects repeated project UUIDs
across pages and enforces local page/response-size ceilings.

## Project device directory

`GET /openapi/v2.0/project/device`

This request sends both `X-User-Token` and the selected `X-Project-Uuid`. The
official contract has no pagination parameters and currently returns at most
1000 topology entries. Each `data.list` entry may contain a `gateway`, a
`drone`, or both:

```json
{
  "code": 0,
  "message": "",
  "data": {
    "list": [
      {
        "gateway": {
          "sn": "<redacted-dock-sn>",
          "callsign": "Sanitized Dock",
          "device_model": {
            "key": "3-2-0",
            "domain": "3",
            "type": "2",
            "sub_type": "0",
            "name": "DJI Dock 2",
            "class": "airport"
          },
          "device_online_status": true,
          "mode_code": 0,
          "camera_list": []
        },
        "drone": {
          "sn": "<redacted-aircraft-sn>",
          "callsign": "",
          "device_model": {
            "key": "0-91-1",
            "domain": "0",
            "type": "91",
            "sub_type": "1",
            "name": "M3TD",
            "class": "drone"
          },
          "device_online_status": false,
          "mode_code": 14,
          "camera_list": null
        }
      }
    ]
  }
}
```

Missing or null optional camera fields are valid. A missing/invalid `data.list`,
an entry without both `gateway` and `drone`, or an over-limit response is a
schema error; synchronization fails closed and does not mark existing devices
missing.

## Error and retry contract

The gateway can report failure as an HTTP error, or as HTTP 200 with non-zero
business `code`. Both layers are evaluated.

| Class | Published example | Connector behavior |
| --- | --- | --- |
| Invalid/expired token | HTTP 401 or business `200401` | Credential invalid; no automatic retry. |
| Project permission denied | HTTP 403 or business `200403` | Scope revoked; no automatic retry. |
| Missing project/resource | HTTP 404 or equivalent non-zero code | Scope unavailable; no automatic retry. |
| Rate limited | HTTP 429, optional `Retry-After`, or business `210429` | Bounded retry honoring `Retry-After`, then backoff. |
| Server failure | HTTP 5xx or business `200500`, `210500`, `210504`, `210318` | Bounded retry with jitter. |
| Malformed/oversized response | N/A | Schema failure; no partial apply. |

The public documentation does not publish a numeric request quota. AeroSight
therefore applies conservative local concurrency limits and treats `429` /
`210429` as authoritative rather than assuming a vendor rate.

## Official references

- FlightHub 2 user manual, Public Cloud OpenAPI V2:
  <https://fh.dji.com/user-manual/cn/custom-development/open-api/public-cloud-v2.html>
- Authentication tutorial:
  <https://s.apifox.cn/4de4a239-c2cc-4572-9b65-90738289f37a>
- Project list:
  <https://s.apifox.cn/4de4a239-c2cc-4572-9b65-90738289f37a/454273364e0>
- Project device list:
  <https://s.apifox.cn/4de4a239-c2cc-4572-9b65-90738289f37a/456680822e0>
- Official OpenAPI V2 example repository:
  <https://github.com/dji-sdk/FlightHub-2-OpenAPI-V2-Demo>
