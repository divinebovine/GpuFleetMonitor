package main

import (
	"log"
	"os"

	"github.com/divinebovine/GpuFleetMonitor/internal/diagnosis"
	"github.com/divinebovine/GpuFleetMonitor/internal/escalation"
	"github.com/divinebovine/GpuFleetMonitor/internal/temporal/activities"
	"github.com/divinebovine/GpuFleetMonitor/internal/temporal/workflows"
	"go.temporal.io/sdk/client"
	"go.temporal.io/sdk/worker"
)

func main() {
	hostPort := os.Getenv("TEMPORAL_ADDRESS")
	if hostPort == "" {
		hostPort = "localhost:7233"
	}
	c, err := client.Dial(client.Options{
		HostPort: hostPort,
	})

	if err != nil {
		log.Fatal(err)
	}
	defer c.Close()

	w := worker.New(c, "gpu-monitor", worker.Options{})

	a := activities.NewActivities(diagnosis.NewStore(), escalation.NewStore())

	w.RegisterWorkflow(workflows.MonitorGPU)
	w.RegisterActivity(a)

	log.Fatal(w.Run(worker.InterruptCh()))
}
