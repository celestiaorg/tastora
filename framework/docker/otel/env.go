package otel

// EnvForService returns OTEL env vars to export traces to the collector.
// protocol: "grpc" or "http" (default grpc). Sets both generic and traces-specific vars,
// and marks exporters as insecure (plaintext) for local/E2E environments.
func EnvForService(serviceName string, collector *Node, protocol string) []string {
    proto := protocol
    if proto != "http" {
        proto = "grpc"
    }
    // Endpoints used by SDKs
    var endpoint string
    if proto == "http" {
        endpoint = collector.HTTPEndpoint() // include scheme for HTTP
    } else {
        endpoint = collector.GRPCEndpoint() // host:port for gRPC
    }
    return []string{
        "OTEL_TRACES_EXPORTER=otlp",
        "OTEL_SERVICE_NAME=" + serviceName,
        // Generic OTLP settings
        "OTEL_EXPORTER_OTLP_PROTOCOL=" + proto,
        "OTEL_EXPORTER_OTLP_ENDPOINT=" + endpoint,
        "OTEL_EXPORTER_OTLP_INSECURE=true",
        // Trace-specific overrides (some SDKs read these instead)
        "OTEL_EXPORTER_OTLP_TRACES_PROTOCOL=" + proto,
        "OTEL_EXPORTER_OTLP_TRACES_ENDPOINT=" + endpoint,
        "OTEL_EXPORTER_OTLP_TRACES_INSECURE=true",
    }
}
