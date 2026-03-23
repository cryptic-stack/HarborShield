# Metrics And Alerting Reference

This guide documents the Prometheus metrics HarborShield exposes today and gives practical alerting guidance for release-quality operations.

Prometheus endpoint:

- `http://localhost/metrics`

If you use the optional Prometheus profile, the default scrape target is already configured in:

- [`deploy/init/prometheus.yml`](c:\Users\JBrown\Documents\Project\s3-platform\deploy\init\prometheus.yml)

## Metric families exposed today

HarborShield currently exposes:

- Go runtime metrics from the Prometheus Go client
- process metrics from the Prometheus Go client
- HarborShield application metrics listed below

## HarborShield application metrics

### `harborshield_requests_total`

Type:

- counter

Labels:

- `route`
- `method`
- `status`

Purpose:

- request volume by route, method, and HTTP outcome

Use it for:

- detecting error spikes
- finding unexpectedly hot routes
- confirming traffic is reaching the service

Example query:

```promql
sum by (route, status) (rate(harborshield_requests_total[5m]))
```

### `harborshield_request_latency_seconds`

Type:

- histogram

Labels:

- `route`
- `method`

Purpose:

- request latency distribution by route

Use it for:

- spotting slow admin or S3 endpoints
- tracking latency regression after releases

Example query:

```promql
histogram_quantile(0.95, sum by (le, route) (rate(harborshield_request_latency_seconds_bucket[5m])))
```

### `harborshield_auth_failures_total`

Type:

- counter

Purpose:

- total authentication failures

Use it for:

- spotting brute-force attempts
- spotting OIDC or admin-login breakage

Example query:

```promql
rate(harborshield_auth_failures_total[5m])
```

### `harborshield_uploads_total`

Type:

- counter

Purpose:

- successful uploaded object count on the S3 plane

Use it for:

- confirming object write activity
- correlating user reports with actual upload traffic

### `harborshield_downloads_total`

Type:

- counter

Purpose:

- successful downloaded object count on the S3 plane

Use it for:

- tracking read-heavy behavior
- correlating customer download activity with traffic spikes

### `harborshield_bytes_transferred_total`

Type:

- counter

Labels:

- `direction`
  - `ingress`
  - `egress`

Purpose:

- byte volume through the object plane

Use it for:

- trending upload versus download bandwidth
- identifying sudden transfer spikes

Example query:

```promql
sum by (direction) (rate(harborshield_bytes_transferred_total[5m]))
```

### `harborshield_worker_jobs_total`

Type:

- counter

Labels:

- `job_type`
- `status`

Purpose:

- worker job execution count by job type and outcome

Use it for:

- finding repeated background-job failures
- confirming repair, malware, eventing, and quota jobs are still running

Example query:

```promql
sum by (job_type, status) (rate(harborshield_worker_jobs_total[15m]))
```

## Recommended dashboards

At minimum, put these panels on an operator dashboard:

1. Request rate by route and status
2. P95 request latency by route
3. Authentication failure rate
4. Upload and download rate
5. Ingress and egress bytes per second
6. Worker job outcome rate by job type
7. Go runtime memory and goroutines
8. Container health from Docker or external host monitoring

## Recommended starter alerts

These are practical first alerts for release-quality operation.

### API unavailable

Signal:

- `/healthz` or `/readyz` failing from your external monitor

Why:

- HarborShield itself does not expose an `up` metric; Prometheus gives you that from the scrape target

Example:

```promql
up{job="harborshield-api"} == 0
```

### Elevated HTTP error rate

Example:

```promql
sum(rate(harborshield_requests_total{status=~"Internal Server Error|Bad Gateway|Service Unavailable"}[5m])) > 0
```

Alternative, broader 5xx pattern if you normalize with external relabeling:

- alert when request outcomes indicate repeated server-side failures

### High request latency

Example:

```promql
histogram_quantile(0.95, sum by (le) (rate(harborshield_request_latency_seconds_bucket{route="/api/v1/auth/login"}[5m]))) > 1
```

Use similar route-specific alerts for:

- `/api/v1/settings`
- `/api/v1/dashboard`
- high-volume S3 routes in front of the API

### Authentication failure spike

Example:

```promql
rate(harborshield_auth_failures_total[5m]) > 1
```

Tune based on your environment:

- low threshold for internal lab use
- higher threshold for internet-exposed environments behind additional auth controls

### Worker job failures

Example:

```promql
sum by (job_type) (rate(harborshield_worker_jobs_total{status="failed"}[15m])) > 0
```

Pay special attention to:

- `event_delivery_retry`
- `malware_scan`
- `storage_repair`
- `storage_rebalance`
- `multipart_cleanup`

### Unexpected zero worker activity

Example:

```promql
sum(rate(harborshield_worker_jobs_total[30m])) == 0
```

Use with care:

- only alert if you expect a live system with normal background activity

## What is not exposed yet

The original roadmap called for a broader metrics surface than HarborShield currently exports.

Not yet exposed as first-class Prometheus metrics:

- bucket count
- bytes stored
- active multipart upload count
- queue depth
- dead-letter delivery count
- degraded placement count
- offline storage node count
- quota-denial count
- malware scan backlog count

Today, those signals are still better obtained from:

- `Dashboard`
- `Health`
- `Storage`
- `Events`
- `Malware`
- the support bundle

For release planning, operators should treat these as known observability gaps rather than silently assuming they exist in Prometheus.

## Practical monitoring stance for `v1.0`

For a broad-release-quality single-node deployment, the minimum recommended monitoring set is:

- external check of `http://localhost/healthz`
- external check of `http://localhost/readyz`
- Prometheus scrape of `/metrics`
- alert on scrape failure
- alert on auth failure spike
- alert on worker failure spike
- alert on route latency regression

For distributed beta deployments, also monitor:

- `Storage` page regularly
- recent storage-related audit events
- support bundle output during node or repair incidents

## Next observability improvements

The highest-value metrics follow-ons are:

1. storage-node health gauges
2. degraded-placement and replica-shortfall gauges
3. queue depth and dead-letter gauges
4. quota-denial counters
5. malware backlog and scan outcome counters
