package injector

import (
	"context"
	"log/slog"
	"os"
	"time"

	"github.com/NVIDIA/go-dcgm/pkg/dcgm"
	"github.com/divinebovine/GpuFleetMonitor/internal/gpu"
)

const (
	Base time.Duration = 10 * time.Second
)

type Ticker struct {
	logger *slog.Logger

	// failing tracks which (entity, field) pairs are currently failing to
	// inject, so a persistent failure (e.g. a dead hostengine connection)
	// logs once on the transition into failure instead of flooding the
	// log on every tick for every field on every GPU.
	failing map[injectKey]bool
}

type injectKey struct {
	entityID uint
	fieldID  FieldID
}

var DefaultTicker = NewTicker()

func NewTicker() *Ticker {
	t := &Ticker{
		logger:  slog.New(slog.NewJSONHandler(os.Stderr, nil)),
		failing: make(map[injectKey]bool),
	}
	return t
}

func (t *Ticker) Run(ctx context.Context, states []GPUState, getCfg func() *gpu.SimulationSettings, recorder MetricsRecorder) error {
	cfg := getCfg()
	speedMultiplier := cfg.SpeedMultiplier
	ticker := time.NewTicker(Base / time.Duration(cfg.SpeedMultiplier))
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			cfg = getCfg()
			currentSpeedMultiplier := cfg.SpeedMultiplier
			if currentSpeedMultiplier != speedMultiplier {
				speedMultiplier = currentSpeedMultiplier
				ticker.Reset(Base / time.Duration(speedMultiplier))
			}

			// advance all GPU states and values
			// TODO: use justFailedHard
			// justFailedHard := Advance(states, cfg)
			Advance(states, cfg)
			AdvanceValues(states)

			// report metrics and inject field values
			for i := range states {
				eID := states[i].EntityID
				for _, fID := range reportableFieldIDs {
					if value, ok := states[i].Values[fID]; ok {
						recorder.Record(eID, fID, value)
						t.injectFieldValue(eID, fID, value)
					}
				}
			}

		case <-ctx.Done():
			return ctx.Err()
		}
	}
}

var reportableFieldIDs = []FieldID{
	FieldMemTemp,
	FieldGPUTemp,
	FieldPowerUsage,
	FieldECCSingle,
	FieldECCDouble,
}

var dcgmFieldIDType = map[FieldID]uint{
	FieldMemTemp:    dcgm.DCGM_FT_DOUBLE,
	FieldGPUTemp:    dcgm.DCGM_FT_DOUBLE,
	FieldPowerUsage: dcgm.DCGM_FT_DOUBLE,
	FieldECCSingle:  dcgm.DCGM_FT_INT64,
	FieldECCDouble:  dcgm.DCGM_FT_INT64,
}

func (t *Ticker) injectFieldValue(entityID uint, fieldID FieldID, value float64) {
	fieldType := dcgmFieldIDType[fieldID]

	// dcgm's InjectFieldValue will panic if the value does not match the field type
	// so a conversion is needed for int64
	var v any
	switch fieldType {
	case dcgm.DCGM_FT_DOUBLE:
		v = value // no conversion needed
	case dcgm.DCGM_FT_INT64:
		v = int64(value)
	}

	err := dcgm.InjectFieldValue(entityID, dcgm.Short(fieldID), fieldType, 0, time.Now().UnixMicro(), v)
	t.logInjectResult(entityID, fieldID, err)
}

// logInjectResult logs InjectFieldValue failures and recoveries, logging only
// on the transition into/out of failure so a persistent failure (e.g. a dead
// hostengine connection) doesn't flood the log on every tick for every field
// on every GPU.
func (t *Ticker) logInjectResult(entityID uint, fieldID FieldID, err error) {
	key := injectKey{entityID: entityID, fieldID: fieldID}

	if err != nil {
		if !t.failing[key] {
			t.failing[key] = true
			t.logger.Warn("InjectFieldValue failed", "entityID", entityID, "fieldID", fieldID, "err", err)
		}
		return
	}

	if t.failing[key] {
		delete(t.failing, key)
		t.logger.Info("InjectFieldValue recovered", "entityID", entityID, "fieldID", fieldID)
	}
}
