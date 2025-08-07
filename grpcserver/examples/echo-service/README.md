# Echo Service Example

This example demonstrates how to use the `grpcserver` package to create a simple gRPC service that echoes messages back to the client.

## What it does

The Echo Service provides two gRPC methods:
- `Echo`: A unary RPC that takes a message and returns it back
- `EchoStream`: A bidirectional streaming RPC that echoes messages in real-time

The service also includes:
- Health check endpoint
- gRPC reflection for debugging
- Prometheus metrics collection
- Structured access logging
- Throttling
  - Rate limiting per client
  - Concurrent request limiting (in-flight limiting)
  - Configurable burst and backlog limits
  - Dry-run mode for testing

Configuration is read from the `config.yml` file, which allows you to customize server settings including logging and throttling rules and limit.

## Starting the service

Run the service using:

```bash
go run main.go
```

You should see log output similar to:
```json
{"level":"info","time":"2025-07-15T17:39:48.235952+03:00","msg":"starting application HTTP server...","pid":44965,"address":":9090","write_timeout":"1m0s","read_timeout":"15s","read_header_timeout":"10s","idle_timeout":"1m0s","shutdown_timeout":"5s"}
{"level":"info","time":"2025-07-15T17:39:48.236104+03:00","msg":"starting gRPC server...","pid":44965,"address":":50051"}
```

The service starts two servers:
- gRPC server on port `:50051`
- HTTP server for metrics on port `:9090`

## Testing with grpcurl

### Echo method (unary RPC)

Test the Echo method with:

```bash
grpcurl -d '{"payload":"hi"}' -plaintext localhost:50051 echo_service.EchoService/Echo
```

Response:
```json
{
  "payload": "hi"
}
```

Log:
```json
{"level":"info","time":"2025-07-15T17:58:47.597963+03:00","msg":"gRPC call finished in 0.000s","pid":23788,"request_id":"d1r6p9vuesdlpr70i700","int_request_id":"d1r6p9vuesdlpr70i70g","trace_id":"","grpc_service":"echo_service.EchoService","grpc_method":"Echo","grpc_method_type":"unary","remote_addr":"[::1]:55177","user_agent":"grpcurl/1.9.2 grpc-go/1.61.0","remote_addr_ip":"::1","remote_addr_port":55177,"grpc_service":"echo_service.EchoService","grpc_method":"Echo","grpc_method_type":"unary","remote_addr":"[::1]:55177","user_agent":"grpcurl/1.9.2 grpc-go/1.61.0","remote_addr_ip":"::1","remote_addr_port":55177,"grpc_code":"OK","duration_ms":0}
```

### EchoStream method (streaming RPC)

Test the EchoStream method with:

```bash
grpcurl -d '{"payload":"hello"}' -plaintext localhost:50051 echo_service.EchoService/EchoStream
```

Response:
```json
{
  "payload": "hello"
}
```

Log:
```json
{"level":"info","time":"2025-07-15T18:00:17.568699+03:00","msg":"gRPC call finished in 0.003s","pid":23788,"request_id":"d1r6q0fuesdlpr70i720","int_request_id":"d1r6q0fuesdlpr70i72g","trace_id":"","grpc_service":"echo_service.EchoService","grpc_method":"EchoStream","grpc_method_type":"stream","remote_addr":"[::1]:55332","user_agent":"grpcurl/1.9.2 grpc-go/1.61.0","remote_addr_ip":"::1","remote_addr_port":55332,"grpc_service":"echo_service.EchoService","grpc_method":"EchoStream","grpc_method_type":"stream","remote_addr":"[::1]:55332","user_agent":"grpcurl/1.9.2 grpc-go/1.61.0","remote_addr_ip":"::1","remote_addr_port":55332,"grpc_code":"OK","duration_ms":2}
```

## Prometheus Metrics

The service automatically collects metrics for:
- gRPC call duration histograms
- Number of calls in flight
- **Throttling metrics** - Rate limiting and in-flight limiting rejections

Access the metrics endpoint with:

```bash
curl localhost:9090/metrics
```

This returns Prometheus-format metrics including:
- `grpc_call_duration_seconds` - Histogram of gRPC call durations
- `grpc_calls_in_flight` - Current number of gRPC calls being served
- `grpc_rate_limit_rejects_total` - Counter of requests rejected due to rate limits
- `grpc_in_flight_limit_rejects_total` - Counter of requests rejected due to in-flight limits

## Throttling Functionality

The echo service demonstrates the throttling capabilities of the `grpcserver/interceptor/throttle` package with **3 simple limitings**:

### 1. Global In-Flight Limiting
Controls the total number of concurrent requests across the entire service (max 1000 concurrent requests globally).

### 2. Rate Limiting by Client ID Header
Rate limits requests based on the `client-id` header:
- 10 requests per second per client ID
- Burst of 5 requests allowed
- Tracks up to 1000 different client IDs

### 3. In-Flight Limiting by Remote Address  
Limits concurrent requests per client IP address:
- Max 100 concurrent requests per IP
- Backlog queue: up to 50 requests wait up to 3 seconds
- Tracks up to 500 different IP addresses

### Single Throttling Rule

All 3 throttling mechanisms are applied to **all Echo service methods** (`/echo_service.EchoService/*`) through a single rule named `echo-service-throttling`.

### Testing Throttling

Send requests with different `client-id` headers to test header-based rate limiting:

```bash
# Client "mobile-app" - should work (within 10/sec limit)
for i in {1..15}; do
  echo "Request $i for mobile-app"
  grpcurl -H "client-id: mobile-app" \
    -d '{"payload":"test"}' -plaintext localhost:50051 echo_service.EchoService/Echo
  sleep 0.1  # Small delay to avoid hitting rate limit too fast
done

# Test burst limit - send 15 requests quickly (part of them should be throttled)
echo "Testing burst limit for client desktop-app:"
for i in {1..15}; do
  grpcurl -H "client-id: desktop-app" \
    -d '{"payload":"burst-test"}' -plaintext localhost:50051 echo_service.EchoService/Echo &
done
wait
```

#### Monitor Throttling in Logs

When throttling occurs, you'll see log entries like:

```json
{"level":"warn","msg":"rate limit exceeded","rate_limit_key":"mobile-app"}
{"level":"info","msg":"gRPC call finished in 0.000s","grpc_code":"ResourceExhaust"}
```

The `RESOURCE_EXHAUSTED` status indicates throttling was applied.

### Monitoring Throttling Metrics

After triggering throttling, you can observe the metrics:

```bash
# Check throttling metrics
curl -s localhost:9090/metrics | grep -E "(rate_limit_rejects|in_flight_limit_rejects)"
```

Example output after throttling:
```
grpc_rate_limit_rejects_total{dry_run="no",rule="echo-service-throttling"} 6
grpc_in_flight_limit_rejects_total{backlogged="no",dry_run="no",rule="echo-service-throttling"} 2
```

### Dry Run Mode

For testing, you can enable dry-run mode in `config.yml` by setting `dryRun: true` in any zone. This will:
- Log throttling decisions without actually blocking requests
- Allow you to see what would be throttled in production
- Useful for capacity planning and testing
