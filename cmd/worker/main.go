package main

import (
	"log"

	"github.com/divinebovine/GpuFleetMonitor/internal/diagnosis"
	"github.com/divinebovine/GpuFleetMonitor/internal/escalation"
	"github.com/divinebovine/GpuFleetMonitor/internal/temporal/activities"
	"github.com/divinebovine/GpuFleetMonitor/internal/temporal/workflows"
	"go.temporal.io/sdk/client"
	"go.temporal.io/sdk/worker"
)

func main() {
	c, err := client.Dial(client.Options{
		HostPort: "localhost:7233",
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
