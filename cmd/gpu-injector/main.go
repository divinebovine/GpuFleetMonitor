package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"syscall"

	"github.com/NVIDIA/go-dcgm/pkg/dcgm"
	"github.com/divinebovine/GpuFleetMonitor/internal/gpu"
	"github.com/divinebovine/GpuFleetMonitor/internal/injector"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"golang.org/x/sync/errgroup"
)

func main() {
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stderr, nil)))

	cleanup, err := dcgm.Init(dcgm.Standalone, "127.0.0.1:5555", "0")
	defer cleanup()

	if err != nil {
		slog.Error("failed initializing dcgm.", "error", err)
		os.Exit(1)
	}
	fmt.Println("dcgm initialized successfully")

	gpuCount, model := getEnvVars()
	entities, err := injector.EnsureFakeEntites(gpuCount)
	if err != nil {
		slog.Error("unable to ensure fake gpus", "error", err)
		os.Exit(1)
	}

	gpuStates := injector.NewGPUStates(entities, model)

	modelByEntity := make(map[uint]string)
	for _, entity := range entities {
		modelByEntity[entity] = model
	}

	smux := http.NewServeMux()
	srv := &http.Server{Addr: ":9400", Handler: smux}
	ticker := injector.NewTicker()

	recorder := injector.NewMetricsRecorder(modelByEntity)
	reg := prometheus.NewRegistry()
	reg.MustRegister(recorder)
	promHandler := promhttp.HandlerFor(reg, promhttp.HandlerOpts{})

	smux.HandleFunc("/metrics", promHandler.ServeHTTP)
	smux.HandleFunc("/healthz", ticker.HealthzHandler)

	g, ctx := errgroup.WithContext(ctx)

	g.Go(func() error {
		<-ctx.Done()
		return srv.Shutdown(context.Background())
	})

	g.Go(srv.ListenAndServe)
	g.Go(func() error {
		return ticker.Run(ctx, gpuStates, gpu.Config.Get, recorder)
	})

	if err := g.Wait(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		log.Fatal(err)
	}
}

func getEnvVars() (int, string) {
	val, exists := os.LookupEnv("GPU_COUNT")
	if !exists {
		slog.Error("GPU_COUNT is not set")
		os.Exit(1)
	}

	gpuCount, err := strconv.Atoi(val)
	if err != nil {
		slog.Error("unable to parse GPU_COUNT, expected an integer", "value", val, "error", err)
		os.Exit(1)
	}

	model, exists := os.LookupEnv("MODEL")
	if !exists {
		slog.Error("MODEL is not set")
		os.Exit(1)
	}

	return gpuCount, model
}
