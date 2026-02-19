package otel

// EnvForService returns a set of common OTEL environment variables for a service
// to export traces to the provided collector. Protocol can be "grpc" or "http" (defaults to grpc).
func EnvForService(serviceName string, collector *Node, protocol string) []string {
    proto := protocol
    if proto != "http" {
        proto = "grpc"
    }
    endpoint := collector.GRPCEndpoint()
    if proto == "http" {
        // For HTTP, the SDKs typically expect the base URL, including scheme
        endpoint = collector.HTTPEndpoint()
    }
    return []string{
        "OTEL_TRACES_EXPORTER=otlp",
        "OTEL_EXPORTER_OTLP_PROTOCOL=" + proto,
        "OTEL_EXPORTER_OTLP_ENDPOINT=" + endpoint,
        "OTEL_SERVICE_NAME=" + serviceName,
    }
}

