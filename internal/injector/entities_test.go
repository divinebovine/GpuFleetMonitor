//go:build integration

package injector

import (
	"context"
	"fmt"
	"os"
	"slices"
	"testing"
	"time"

	"github.com/NVIDIA/go-dcgm/pkg/dcgm"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"
)

var hostengineAddr string

func TestMain(m *testing.M) {
	ctx, cancel := context.WithDeadline(context.Background(), time.Now().Add(30*time.Second))
	defer cancel()

	container, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: testcontainers.ContainerRequest{
			Image:        "nvidia/dcgm:4.5.2-1-ubuntu22.04",
			ExposedPorts: []string{"5555/tcp"},
			Cmd:          []string{"-n", "-b", "0.0.0.0", "--log-level", "INFO", "-f", "-"},
			WaitingFor:   wait.ForListeningPort("5555/tcp"),
		},
		Started: true,
	})

	if err != nil {
		fmt.Fprintln(os.Stderr, "starting hostengine container:", err)
		os.Exit(1)
	}

	host, _ := container.Host(ctx)
	port, _ := container.MappedPort(ctx, "5555/tcp")
	hostengineAddr = fmt.Sprintf("%s:%s", host, port.Port())

	code := m.Run()

	container.Terminate(ctx)
	os.Exit(code)
}

func TestEnsureFakeEntitiesIdempotency(t *testing.T) {
	cleanup, err := dcgm.Init(dcgm.Standalone, hostengineAddr, "0")
	defer cleanup()

	if err != nil {
		t.Fatalf("err initializing dcgm. err: %v", err)
		return
	}

	gpuCount := 10
	eIds1, err := EnsureFakeEntites(gpuCount)
	if err != nil {
		t.Fatalf("failed creating entities. err: %v", err)
	}

	eIds2, err := EnsureFakeEntites(gpuCount)
	if err != nil {
		t.Fatalf("failed creating entities. err: %v", err)
	}

	if !slices.Equal(eIds1, eIds2) {
		t.Errorf("eIds1 does not equal eIds2.\n  eIds1: %v\n  eIds2: %v", eIds1, eIds2)
	}
}
