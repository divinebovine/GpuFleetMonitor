package workflows

import (
	"time"

	"github.com/divinebovine/GpuFleetMonitor/internal/diagnosis"
	"github.com/divinebovine/GpuFleetMonitor/internal/gpu"
	"github.com/divinebovine/GpuFleetMonitor/internal/temporal/activities"
	"go.temporal.io/sdk/workflow"
)

func MonitorGPU(ctx workflow.Context, gpuID string) error {
	ao := workflow.ActivityOptions{
		StartToCloseTimeout: 10 * time.Second,
	}
	ctx = workflow.WithActivityOptions(ctx, ao)

	var a *activities.Activities // nil is fine - Temporal uses this for routing only

	var h *gpu.GPUHealth
	hErr := workflow.ExecuteActivity(ctx, a.GetHealth, gpuID).Get(ctx, &h)
	if hErr != nil {
		return hErr
	}

	var d *diagnosis.Diagnosis
	dErr := workflow.ExecuteActivity(ctx, a.Diagnose, h).Get(ctx, &d)
	if dErr != nil {
		return dErr
	}

	if d.Severity == diagnosis.SeverityCritical {
		eErr := workflow.ExecuteActivity(ctx, a.Escalate, d).Get(ctx, nil)
		if eErr != nil {
			return eErr
		}
	}

	return nil
}
