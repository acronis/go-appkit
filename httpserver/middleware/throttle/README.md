# HTTP middleware for API throttling

The package provides two types of server-side throttling for HTTP services:
1. In-flight throttling limits the total number of currently served (in-flight) HTTP requests.
2. Rate-limit throttling limits the rate of HTTP requests using the leaky bucket or the sliding window algorithms (it's configurable, the leaky bucket is used by default).

Throttling is implemented as a typical Go HTTP middleware and can be configured from the code or from the JSON/YAML configuration file.

Please see [testable example](./example_test.go) to understand how to configure and use the middleware.

## Throttling configuration

The throttling configuration is usually stored in the JSON or YAML file. The configuration consists of the following parts:
1. `rateLimitZones`. Each zone has a rate limit, burst limit, and other parameters.
2. `inFlightLimitZones`. Each zone has an in-flight limit, backlog limit, and other parameters.
3. `rules`. Each rule contains a list of routes and rate/in-flight limiting zones that should be applied to these routes.

### Global throttling example

Global throttling assesses all traffic coming into an API from all sources and ensures that the overall rate/concurrency limit is not exceeded.
Overwhelming an endpoint with traffic is an easy and efficient way to carry out a denial-of-service attack.
By using a global rate/concurrency limit, you can ensure that all incoming requests are within a specific limit.

#### API-level throttling
```yaml
rateLimitZones:
  rl_total:
    rateLimit: 5000/s
    burstLimit: 10000
    responseStatusCode: 503
    responseRetryAfter: 5s

inFlightLimitZones:
  ifl_total:
    inFlightLimit: 5000
    backlogLimit: 10000
    backlogTimeout: 30s
    responseStatusCode: 503
    responseRetryAfter: 1m

rules:
  - routes:
      - path: "/"
    rateLimits:
      - zone: rl_total
    inFlightLimits:
      - zone: ifl_total
```

With this configuration, all HTTP requests will be limited by rate (no more than 5000 requests per second, rateLimit). Excessive requests within the burst limit (10000 here, `burstLimit`) will be served immediately regardless of the specified rate, requests above the burst limit will be rejected with the 503 error (`responseStatusCode`), and `"Retry-After: 5"` HTTP header (`responseRetryAfter`).

Additionally, there is a concurrency limit. If 5000 requests (`inFlightLimit`) are being processed right now, new incoming requests will be backlogged (suspended).
If there are more than 10000 such backlogged requests (`backlogLimit`), the rest will be rejected immediately. The request can be in backlogged status for no more than 30 seconds (`backlogTimeout`) and then it will be rejected. Response for rejected request contains 503 (`responseStatusCode`) HTTP error code and `"Retry-After: 60"` HTTP header (`responseRetryAfter`).

`backlogLimit` and `backlogTimeout` can be specified for rate-limiting zones too.

### Per-client throttling example

Per-client throttling is focused on controlling traffic from individual sources and making sure that API clients are staying within their prescribed limits. Per-client throttling allows avoiding cases when one client exhausts the application resources of the entire backend service (for example, it uses all connections from the DB pool), and all other clients have to wait for their release.

To implement per-client throttling, the package uses the concept of "identity". If the client is identified by a unique key, the package can throttle requests per this key.
`MiddlewareOpts` struct has a `GetKeyIdentity` callback that should return the key for the current request. It may be a user ID, JWT "sub" claim, or any other unique identifier.
If any rate/in-flight limiting zone's `key.type` field is set to `identity`, the `GetKeyIdentity` callback must be implemented.

Example of per-client throttling configuration:

```yaml
rateLimitZones:
  rl_identity:
    rateLimit: 50/s
    burstLimit: 100
    responseStatusCode: 429
    responseRetryAfter: auto
    key:
      type: identity
    maxKeys: 50000

inFlightLimitZones:
  ifl_identity:
    inFlightLimit: 64
    backlogLimit: 128
    backlogTimeout: 30s
    responseStatusCode: 429
    key:
      type: identity
    maxKeys: 50000

  ifl_identity_expensive_op:
    inFlightLimit: 4
    backlogLimit: 8
    backlogTimeout: 30s
    responseStatusCode: 429
    key:
      type: identity
    maxKeys: 50000
    excludedKeys:
      - "150853ab-322c-455d-9793-8d71bf6973d9" # Exclude root admin.

rules:
  - routes:
      - path: "/"
    rateLimits:
      - zone: rl_identity
    inFlightLimits:
      - zone: ifl_identity
    alias: per_identity
  - routes:
      - path: "= /api/v1/do_expensive_op_1"
        methods: POST
      - path: "= /api/v1/do_expensive_op_2"
        methods: POST
    inFlightLimits:
      - zone: ifl_identity_expensive_op
    alias: per_identity_expensive_ops
```

All throttling counters are stored inside an in-memory LRU cache (`maxKeys` determines its size).

For the rate-limiting zone, `responseRetryAfter` may be specified as `"auto"`. In this case, the time when a client may retry the request will be calculated automatically.

Each throttling rule may contain an unlimited number of rate/in-flight limiting zones. All rule zones will be applied to all specified routes. The route is described as path + list of HTTP methods. To select a route, exactly the same algorithm is used as to select a location in Nginx (http://nginx.org/en/docs/http/ngx_http_core_module.html#location). Also, the route may have an alias that will be used in the Prometheus metrics label (see example below).

### Sliding window rate-limiting

```yaml
rateLimitZones:
  rl_identity:
    alg: sliding_window
    rateLimit: 15/m
    responseStatusCode: 429
    responseRetryAfter: auto
    key:
      type: identity
    maxKeys: 50000

rules:
  - routes:
      - path: "/"
    rateLimits:
      - zone: rl_identity
    alias: per_identity
```

In this example sliding window algorithm will be used for rate-limiting (`alg` parameter has `"token_bucket"` value by default).  It means, only 15 requests are allowed per minute. They could be sent even simultaneously, but all exceeding requests that are received in the same minute will be rejected.

### Example of throttling of all requests with a "bad" User-Agent HTTP header

```yaml
rateLimitZones:
  rl_bad_user_agents:
    rateLimit: 500/s
    burstLimit: 1000
    responseStatusCode: 503
    responseRetryAfter: 15s
    key:
      type: header
      headerName: "User-Agent"
      noBypassEmpty: true
    includedKeys:
      - ""
      - "Go-http-client/1.1"
      - "python-requests/*"
      - "Python-urllib/*"
    maxKeys: 1000

rules:
  - routes:
      - path: "/"
    rateLimits:
      - zone: rl_bad_user_agents
```

### Throttle requests by remote address example

```yaml
rateLimitZones:
  rl_by_remote_addr:
    rateLimit: 100/s
    burstLimit: 1000
    responseStatusCode: 503
    responseRetryAfter: auto
    key:
      type: remote_addr
    maxKeys: 10000

rules:
  - routes:
      - path: "/"
    rateLimits:
      - zone: rl_by_remote_addr
```

## Prometheus metrics

The package collects several metrics in the Prometheus format:
 - `rate_limit_rejects_total`. Type: counter; Labels: dry_run, rule.
 - `in_flight_limit_rejects_total`. Type: counter; Labels: dry_run, rule, backlogged.

## Tags

Tags are useful when different rules of the same configuration should be used by different middlewares. For example, suppose you want to have two different throttling rules:

 1. A rule for all requests.
 2. A rule for all identity-aware (authorized) requests.

```yaml
# ...
rules:
  - routes:
    - path: "/hello"
      methods: GET
    rateLimits:
      - zone: rl_zone1
    tags: all_reqs

  - routes:
    - path: "/feedback"
      methods: POST
    inFlightLimits:
      - zone: ifl_zone1
    tags: all_reqs

  - routes:
    - path: /api/1/users
      methods: PUT
    rateLimits:
      - zone: rl_zone2
    tags: require_auth_reqs
# ...
```

In your code, you will have two middlewares that will be executed at different steps of the HTTP request serving process. Each middleware should only apply its own throttling rule.

```go
allMw := MiddlewareWithOpts(cfg, "my-app-domain", throttleMetrics, MiddlewareOpts{Tags: []string{"all_reqs"}})
requireAuthMw := MiddlewareWithOpts(cfg, "my-app-domain", throttleMetrics, MiddlewareOpts{Tags: []string{"require_auth_reqs"}})
```

## Dry-run mode

Before configuring real-life throttling, usually, it's a good idea to try the dry-run mode. It doesn't affect the processing requests flow, however, all excessive requests are still counted and logged. Dry-run mode allows you to better understand how your API is used and determine the right throttling parameters.

The dry-run mode can be enabled using the `dryRun` configuration parameter. Example:
```yaml
rateLimitZones:
  rl_identity:
    rateLimit: 50/s
    burstLimit: 100
    responseStatusCode: 429
    responseRetryAfter: auto
    key:
      type: identity
    maxKeys: 50000
    dryRun: true

inFlightLimitZones:
  ifl_identity:
    inFlightLimit: 64
    backlogLimit: 128
    backlogTimeout: 30s
    responseStatusCode: 429
    key:
      type: identity
    maxKeys: 50000
    dryRun: true

rules:
  - routes:
      - path: "/"
    rateLimits:
      - zone: rl_identity
    inFlightLimits:
      - zone: ifl_identity
    alias: per_identity
```

If specified limits are exceeded, the corresponding messages will be logged.

For rate-limiting:

```json
{"msg": "too many requests, serving will be continued because of dry run mode", "rate_limit_key": "ee9a0dd8-7396-5478-8b83-ab7402d6746b"}
```

For in-flight limiting:
```json
{"msg": "too many in-flight requests, serving will be continued because of dry run mode", "in_flight_limit_key": "3c00e780-5721-59f8-acad-f0bf719777d4"}
```

## License

Copyright © 2024 Acronis International GmbH.

Licensed under [MIT License](./../../../LICENSE).
