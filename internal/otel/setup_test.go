package otelsetup

import (
    "context"
    "testing"
    "github.com/stretchr/testify/require"
)

func TestSetupAndShutdown(t *testing.T) {
    shutdown, err := Setup(context.Background())
    require.NoError(t, err)
    require.NotNil(t, shutdown)
    require.NoError(t, shutdown(context.Background()))
}
