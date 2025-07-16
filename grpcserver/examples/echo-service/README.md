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
- Structured logging

Configuration is read from the `config.yml` file, which allows you to customize server settings such as address, timeouts, and logging options.

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

Access the metrics endpoint with:

```bash
curl localhost:9090
```

This returns Prometheus-format metrics including:
- `grpc_call_duration_seconds` - Histogram of gRPC call durations
- `grpc_calls_in_flight` - Current number of gRPC calls being served
