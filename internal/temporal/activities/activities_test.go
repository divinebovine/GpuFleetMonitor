package activities_test

import (
	"testing"

	"github.com/divinebovine/GpuFleetMonitor/internal/diagnosis"
	"github.com/divinebovine/GpuFleetMonitor/internal/escalation"
	"github.com/divinebovine/GpuFleetMonitor/internal/gpu"
	"github.com/divinebovine/GpuFleetMonitor/internal/temporal/activities"
	"github.com/stretchr/testify/suite"
	"go.temporal.io/sdk/testsuite"
)

type ActivitiesTestSuite struct {
	suite.Suite
	testsuite.WorkflowTestSuite
}

func (s *ActivitiesTestSuite) newActivities() (*activities.Activities, *diagnosis.Store, *escalation.Store) {
	ds := diagnosis.NewStore()
	es := escalation.NewStore()
	return activities.NewActivities(ds, es), ds, es
}

func (s *ActivitiesTestSuite) TestGetHealthReturnsHealth() {
	env := s.NewTestActivityEnvironment()
	a, _, _ := s.newActivities()
	env.RegisterActivity(a)

	val, err := env.ExecuteActivity(a.GetHealth, "GPU-00001")
	s.NoError(err)

	var h *gpu.GPUHealth
	s.NoError(val.Get(&h))
	s.Equal("GPU-00001", h.GPUID)
}

func (s *ActivitiesTestSuite) TestGetHealthInvalidID() {
	env := s.NewTestActivityEnvironment()
	a, _, _ := s.newActivities()
	env.RegisterActivity(a)

	_, err := env.ExecuteActivity(a.GetHealth, "INVALID")
	s.Error(err)
}

func (s *ActivitiesTestSuite) TestDiagnoseReturnsDiagnosisAndSavesToStore() {
	env := s.NewTestActivityEnvironment()
	a, ds, _ := s.newActivities()
	env.RegisterActivity(a)

	h := &gpu.GPUHealth{
		GPUID:       "GPU-00001",
		NodeID:      "NODE-0001",
		Utilization: 50.0,
		Temperature: gpu.Temperature{
			GPUCoreCelsius:    75.0,
			MemoryCelsius:     60.0,
			GPUCoreWarningThreshold:  83.0,
			GPUCoreCriticalThreshold: 87.0,
		},
		Memory: gpu.Memory{
			TotalBytes:  80 * 1024 * 1024 * 1024,
			UsedBytes:   10 * 1024 * 1024 * 1024,
			FreeBytes:   70 * 1024 * 1024 * 1024,
			Utilization: 12.5,
		},
		Power: gpu.Power{
			DrawWatts:   300.0,
			LimitWatts:  700.0,
			Utilization: 42.0,
		},
	}

	val, err := env.ExecuteActivity(a.Diagnose, h)
	s.NoError(err)

	var d *diagnosis.Diagnosis
	s.NoError(val.Get(&d))
	s.Equal("GPU-00001", d.GPUID)

	saved, ok := ds.GetByID(d.ID)
	s.True(ok)
	s.Equal(d.ID, saved.ID)
}

func (s *ActivitiesTestSuite) TestEscalateSavesToStore() {
	env := s.NewTestActivityEnvironment()
	a, _, es := s.newActivities()
	env.RegisterActivity(a)

	d := &diagnosis.Diagnosis{
		ID:       "diag-GPU-00005",
		GPUID:    "GPU-00005",
		Severity: diagnosis.SeverityCritical,
	}

	val, err := env.ExecuteActivity(a.Escalate, d)
	s.NoError(err)

	var e *escalation.Escalation
	s.NoError(val.Get(&e))
	s.Equal("GPU-00005", e.GPUID)
	s.Equal("diag-GPU-00005", e.DiagnosisID)
	s.Equal(string(diagnosis.SeverityCritical), e.Severity)
	s.Equal(escalation.StatusOpen, e.Status)

	saved, ok := es.GetByID(e.ID)
	s.True(ok)
	s.Equal(e.ID, saved.ID)
}

func TestActivitiesTestSuite(t *testing.T) {
	suite.Run(t, new(ActivitiesTestSuite))
}
