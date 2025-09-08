package config

import (
    "flag"
    "time"
)

// Config holds instance-level configuration for the service.
type Config struct {
    ListenAddr            string
    MaxReceiveMessageSize int

    AttributeKey    string
    Window          time.Duration
    MaxQueue        int
    OutputFormat    string
    LogLevel        string
    GracefulTimeout time.Duration
}

// RegisterFlags registers CLI flags and returns a reader that captures them after flag.Parse().
func RegisterFlags() func() Config {
    listenAddr := flag.String("listenAddr", "localhost:4317", "The listen address")
    maxRecv := flag.Int("maxReceiveMessageSize", 16*1024*1024, "The max message size in bytes the server can receive")

    attrKey := flag.String("attributeKey", "foo", "Attribute key to aggregate on")
    window := flag.Duration("window", 10*time.Second, "Aggregation window duration")
    maxQueue := flag.Int("maxQueue", 100_000, "Max ingestion queue size")
    outFmt := flag.String("outputFormat", "json", "Output format: json|log")
    logLevel := flag.String("logLevel", "info", "Log level: debug|info|warn|error")
    graceful := flag.Duration("gracefulTimeout", 10*time.Second, "Graceful shutdown timeout")

    return func() Config {
        return Config{
            ListenAddr:            *listenAddr,
            MaxReceiveMessageSize: *maxRecv,
            AttributeKey:          *attrKey,
            Window:                *window,
            MaxQueue:              *maxQueue,
            OutputFormat:          *outFmt,
            LogLevel:              *logLevel,
            GracefulTimeout:       *graceful,
        }
    }
}

