package otel

// MinimalLoggingConfigMap returns a minimal collector config as a generic map.
// - Receives OTLP over gRPC (4317) and HTTP (4318)
// - Batches
// - Logs traces to stdout
func MinimalLoggingConfigMap() map[string]any {
    return map[string]any{
        "receivers": map[string]any{
            "otlp": map[string]any{
                "protocols": map[string]any{
                    "grpc": map[string]any{},
                    "http": map[string]any{},
                },
            },
        },
        "exporters": map[string]any{
            // 'logging' exporter deprecated; use 'debug'
            "debug": map[string]any{
                "verbosity": "basic",
            },
        },
        "processors": map[string]any{
            "batch": map[string]any{},
        },
        "service": map[string]any{
            "telemetry": map[string]any{
                "logs": map[string]any{"level": "warn"},
                // New-style telemetry metrics config (Collector 0.146+): readers + prometheus exporter
                "metrics": map[string]any{
                    "readers": []any{
                        map[string]any{
                            "pull": map[string]any{
                                "exporter": map[string]any{
                                    "prometheus": map[string]any{
                                        "host": "0.0.0.0",
                                        "port": 8888,
                                    },
                                },
                            },
                        },
                    },
                },
            },
            "pipelines": map[string]any{
                "traces": map[string]any{
                    "receivers":  []string{"otlp"},
                    "processors": []string{"batch"},
                    "exporters":  []string{"debug"},
                },
            },
        },
    }
}
