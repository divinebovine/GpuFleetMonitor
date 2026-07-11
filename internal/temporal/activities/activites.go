package activities

import (
	"context"
	"fmt"
	"time"

	"github.com/divinebovine/GpuFleetMonitor/internal/diagnosis"
	"github.com/divinebovine/GpuFleetMonitor/internal/escalation"
	"github.com/divinebovine/GpuFleetMonitor/internal/gpu"
)

type Activities struct {
	diagnosisStore  *diagnosis.Store
	escalationStore *escalation.Store
}

func NewActivities(ds *diagnosis.Store, es *escalation.Store) *Activities {
	return &Activities{diagnosisStore: ds, escalationStore: es}
}

func (a *Activities) GetHealth(ctx context.Context, id string) (*gpu.GPUHealth, error) {
	return gpu.GetHealth(ctx, id)
}

func (a *Activities) Diagnose(ctx context.Context, h *gpu.GPUHealth) (*diagnosis.Diagnosis, error) {
	d := diagnosis.Analyze(h, time.Now().UTC())
	a.diagnosisStore.Save(d)

	return d, nil
}

func (a *Activities) Escalate(ctx context.Context, d *diagnosis.Diagnosis) (*escalation.Escalation, error) {
	e := escalation.Escalation{
		ID:          fmt.Sprintf("esc-%s", d.GPUID),
		GPUID:       d.GPUID,
		DiagnosisID: d.ID,
		Severity:    string(d.Severity), // TODO revist and consider unify on type
		Status:      escalation.StatusOpen,
		CreatedAt:   time.Now().UTC(),
	}

	a.escalationStore.Save(e)
	return &e, nil
}
