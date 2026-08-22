package injector

import (
	"strings"
	"testing"

	"github.com/prometheus/client_golang/prometheus/testutil"
)

func TestRecord(t *testing.T) {
	modelByEntity := map[uint]string{
		0: "H100",
		1: "A100",
		2: "V100",
		3: "A30",
	}
	recorder := NewMetricsRecorder(modelByEntity)
	recorder.Record(0, 155, 700) // DCGM_FI_DEV_BOARD_POWER_WATTS 155
	recorder.Record(1, 150, 67)  // DCGM_FI_DEV_GPU_TEMP 150
	recorder.Record(2, 140, 90)  // DCGM_FI_DEV_MEMORY_TEMP_CELSIUS 140
	recorder.Record(3, 310, 42)  // DCGM_FI_DEV_ECC_SBE_VOL_TOTAL 310
	recorder.Record(0, 311, 99)  // DCGM_FI_DEV_ECC_DBE_VOL_TOTAL 311

	expected := strings.NewReader(`# HELP DCGM_FI_DEV_BOARD_POWER_WATTS Current power usage in watts
# TYPE DCGM_FI_DEV_BOARD_POWER_WATTS gauge
DCGM_FI_DEV_BOARD_POWER_WATTS{gpu="0",modelName="H100"} 700
`)
	if err := testutil.CollectAndCompare(recorder, expected, "DCGM_FI_DEV_BOARD_POWER_WATTS"); err != nil {
		t.Error(err)
	}

	expected = strings.NewReader(`# HELP DCGM_FI_DEV_GPU_TEMP Current GPU temperature
# TYPE DCGM_FI_DEV_GPU_TEMP gauge
DCGM_FI_DEV_GPU_TEMP{gpu="1",modelName="A100"} 67
	`)
	if err := testutil.CollectAndCompare(recorder, expected, "DCGM_FI_DEV_GPU_TEMP"); err != nil {
		t.Error(err)
	}

	expected = strings.NewReader(`# HELP DCGM_FI_DEV_MEMORY_TEMP_CELSIUS Current memory temperature
# TYPE DCGM_FI_DEV_MEMORY_TEMP_CELSIUS gauge
DCGM_FI_DEV_MEMORY_TEMP_CELSIUS{gpu="2",modelName="V100"} 90
	`)
	if err := testutil.CollectAndCompare(recorder, expected, "DCGM_FI_DEV_MEMORY_TEMP_CELSIUS"); err != nil {
		t.Error(err)
	}

	expected = strings.NewReader(`# HELP DCGM_FI_DEV_ECC_SBE_VOL_TOTAL Total single bit volatile ECC errors
# TYPE DCGM_FI_DEV_ECC_SBE_VOL_TOTAL counter
DCGM_FI_DEV_ECC_SBE_VOL_TOTAL{gpu="3",modelName="A30"} 42
	`)
	if err := testutil.CollectAndCompare(recorder, expected, "DCGM_FI_DEV_ECC_SBE_VOL_TOTAL"); err != nil {
		t.Error(err)
	}

	expected = strings.NewReader(`# HELP DCGM_FI_DEV_ECC_DBE_VOL_TOTAL Total double bit volatile ECC errors
# TYPE DCGM_FI_DEV_ECC_DBE_VOL_TOTAL counter
DCGM_FI_DEV_ECC_DBE_VOL_TOTAL{gpu="0",modelName="H100"} 99
`)
	if err := testutil.CollectAndCompare(recorder, expected, "DCGM_FI_DEV_ECC_DBE_VOL_TOTAL"); err != nil {
		t.Error(err)
	}
}

func TestRecordReflectsCurrentValueAcrossRepeats(t *testing.T) {
	modelByEntity := map[uint]string{
		0: "H100",
	}
	recorder := NewMetricsRecorder(modelByEntity)
	recorder.Record(0, 310, 42)

	expected := strings.NewReader(`# HELP DCGM_FI_DEV_ECC_SBE_VOL_TOTAL Total single bit volatile ECC errors
# TYPE DCGM_FI_DEV_ECC_SBE_VOL_TOTAL counter
DCGM_FI_DEV_ECC_SBE_VOL_TOTAL{gpu="0",modelName="H100"} 42
	`)
	if err := testutil.CollectAndCompare(recorder, expected, "DCGM_FI_DEV_ECC_SBE_VOL_TOTAL"); err != nil {
		t.Error(err)
	}

	// should not increase
	recorder.Record(0, 310, 42)

	expected = strings.NewReader(`# HELP DCGM_FI_DEV_ECC_SBE_VOL_TOTAL Total single bit volatile ECC errors
# TYPE DCGM_FI_DEV_ECC_SBE_VOL_TOTAL counter
DCGM_FI_DEV_ECC_SBE_VOL_TOTAL{gpu="0",modelName="H100"} 42
	`)
	if err := testutil.CollectAndCompare(recorder, expected, "DCGM_FI_DEV_ECC_SBE_VOL_TOTAL"); err != nil {
		t.Error(err)
	}
}

func TestRecordReflectsDecreasedValueDirectly(t *testing.T) {
	modelByEntity := map[uint]string{
		0: "H100",
	}
	recorder := NewMetricsRecorder(modelByEntity)
	recorder.Record(0, 310, 42)

	expected := strings.NewReader(`# HELP DCGM_FI_DEV_ECC_SBE_VOL_TOTAL Total single bit volatile ECC errors
# TYPE DCGM_FI_DEV_ECC_SBE_VOL_TOTAL counter
DCGM_FI_DEV_ECC_SBE_VOL_TOTAL{gpu="0",modelName="H100"} 42
	`)
	if err := testutil.CollectAndCompare(recorder, expected, "DCGM_FI_DEV_ECC_SBE_VOL_TOTAL"); err != nil {
		t.Error(err)
	}

	recorder.Record(0, 310, 13)

	expected = strings.NewReader(`# HELP DCGM_FI_DEV_ECC_SBE_VOL_TOTAL Total single bit volatile ECC errors
# TYPE DCGM_FI_DEV_ECC_SBE_VOL_TOTAL counter
DCGM_FI_DEV_ECC_SBE_VOL_TOTAL{gpu="0",modelName="H100"} 13
	`)
	if err := testutil.CollectAndCompare(recorder, expected, "DCGM_FI_DEV_ECC_SBE_VOL_TOTAL"); err != nil {
		t.Error(err)
	}
}
