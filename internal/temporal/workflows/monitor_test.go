package workflows_test

import (
	"testing"

	"github.com/divinebovine/GpuFleetMonitor/internal/diagnosis"
	"github.com/divinebovine/GpuFleetMonitor/internal/escalation"
	"github.com/divinebovine/GpuFleetMonitor/internal/gpu"
	"github.com/divinebovine/GpuFleetMonitor/internal/temporal/activities"
	"github.com/divinebovine/GpuFleetMonitor/internal/temporal/workflows"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/suite"
	"go.temporal.io/sdk/temporal"
	"go.temporal.io/sdk/testsuite"
)

type MonitorWorkflowTestSuite struct {
	suite.Suite
	testsuite.WorkflowTestSuite
	env *testsuite.TestWorkflowEnvironment
}

func (s *MonitorWorkflowTestSuite) SetupTest() {
	s.env = s.NewTestWorkflowEnvironment()
}

func (s *MonitorWorkflowTestSuite) AfterTest(_, _ string) {
	s.env.AssertExpectations(s.T())
}

// a is a nil *Activities used only for method references — Temporal routes by name.
var a *activities.Activities

func (s *MonitorWorkflowTestSuite) TestCriticalGPUEscalates() {
	health := &gpu.GPUHealth{GPUID: "GPU-00005", HealthStatus: gpu.StatusCritical}
	diag := &diagnosis.Diagnosis{ID: "diag-GPU-00005", GPUID: "GPU-00005", Severity: diagnosis.SeverityCritical}
	esc := &escalation.Escalation{ID: "esc-GPU-00005", GPUID: "GPU-00005"}

	s.env.OnActivity(a.GetHealth, mock.Anything, "GPU-00005").Return(health, nil)
	s.env.OnActivity(a.Diagnose, mock.Anything, mock.Anything).Return(diag, nil)
	s.env.OnActivity(a.Escalate, mock.Anything, mock.Anything).Return(esc, nil)

	s.env.ExecuteWorkflow(workflows.MonitorGPU, "GPU-00005")

	s.True(s.env.IsWorkflowCompleted())
	s.NoError(s.env.GetWorkflowError())
}

func (s *MonitorWorkflowTestSuite) TestHealthyGPUSkipsEscalation() {
	health := &gpu.GPUHealth{GPUID: "GPU-00001", HealthStatus: gpu.StatusHealthy}
	diag := &diagnosis.Diagnosis{ID: "diag-GPU-00001", GPUID: "GPU-00001", Severity: diagnosis.SeverityLow}

	s.env.OnActivity(a.GetHealth, mock.Anything, "GPU-00001").Return(health, nil)
	s.env.OnActivity(a.Diagnose, mock.Anything, mock.Anything).Return(diag, nil)
	// Escalate is intentionally not mocked — calling it would fail the test

	s.env.ExecuteWorkflow(workflows.MonitorGPU, "GPU-00001")

	s.True(s.env.IsWorkflowCompleted())
	s.NoError(s.env.GetWorkflowError())
}

func (s *MonitorWorkflowTestSuite) TestGetHealthErrorAbortsWorkflow() {
	s.env.OnActivity(a.GetHealth, mock.Anything, "GPU-99999").Return((*gpu.GPUHealth)(nil), temporal.NewNonRetryableApplicationError("GPU not found", "GPUNotFound", nil))

	s.env.ExecuteWorkflow(workflows.MonitorGPU, "GPU-99999")

	s.True(s.env.IsWorkflowCompleted())
	s.Error(s.env.GetWorkflowError())
}

func (s *MonitorWorkflowTestSuite) TestDiagnoseErrorAbortsWorkflow() {
	health := &gpu.GPUHealth{GPUID: "GPU-00001", HealthStatus: gpu.StatusHealthy}

	s.env.OnActivity(a.GetHealth, mock.Anything, "GPU-00001").Return(health, nil)
	s.env.OnActivity(a.Diagnose, mock.Anything, mock.Anything).Return((*diagnosis.Diagnosis)(nil), temporal.NewNonRetryableApplicationError("diagnosis failed", "DiagnosisFailed", nil))

	s.env.ExecuteWorkflow(workflows.MonitorGPU, "GPU-00001")

	s.True(s.env.IsWorkflowCompleted())
	s.Error(s.env.GetWorkflowError())
}

func TestMonitorWorkflowTestSuite(t *testing.T) {
	suite.Run(t, new(MonitorWorkflowTestSuite))
}
