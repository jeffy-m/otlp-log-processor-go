package config

import (
    "flag"
    "testing"
    "time"
    "github.com/stretchr/testify/require"
)

func TestRegisterFlags_Defaults(t *testing.T) {
    // Use a fresh FlagSet to avoid interfering with global flags in other tests.
    orig := flag.CommandLine
    flag.CommandLine = flag.NewFlagSet("test", flag.ContinueOnError)
    t.Cleanup(func(){ flag.CommandLine = orig })

    read := RegisterFlags()
    // Parse no args -> defaults
    _ = flag.CommandLine.Parse([]string{})
    cfg := read()

    require.Equal(t, "localhost:4317", cfg.ListenAddr)
    require.Equal(t, 16*1024*1024, cfg.MaxReceiveMessageSize)
    require.NotEmpty(t, cfg.AttributeKey)
    require.Greater(t, cfg.Window, time.Duration(0))
}

func TestRegisterFlags_Overrides(t *testing.T) {
    orig := flag.CommandLine
    flag.CommandLine = flag.NewFlagSet("test", flag.ContinueOnError)
    t.Cleanup(func(){ flag.CommandLine = orig })

    read := RegisterFlags()
    args := []string{
        "-listenAddr", "0.0.0.0:5000",
        "-maxReceiveMessageSize", "1024",
        "-attributeKey", "bar",
        "-window", "250ms",
        "-maxQueue", "42",
        "-outputFormat", "log",
        "-logLevel", "debug",
        "-gracefulTimeout", "2s",
    }
    require.NoError(t, flag.CommandLine.Parse(args))

    cfg := read()
    require.Equal(t, "0.0.0.0:5000", cfg.ListenAddr)
    require.Equal(t, 1024, cfg.MaxReceiveMessageSize)
    require.Equal(t, "bar", cfg.AttributeKey)
    require.Equal(t, 250*time.Millisecond, cfg.Window)
    require.Equal(t, 42, cfg.MaxQueue)
    require.Equal(t, "log", cfg.OutputFormat)
    require.Equal(t, "debug", cfg.LogLevel)
    require.Equal(t, 2*time.Second, cfg.GracefulTimeout)
}
