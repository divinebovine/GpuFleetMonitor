package gpu

import (
	"context"
	"log/slog"
	"math/rand/v2"
	"time"
)

const (
	Base time.Duration = 10 * time.Second
)

type Ticker struct {
}

var DefaultTicker = NewTicker()

func NewTicker() *Ticker {
	t := &Ticker{}
	return t
}

func (t *Ticker) Start(ctx context.Context) {
	go func() {
		cfg := Config.Get()
		speedMultiplier := cfg.SpeedMultiplier
		ticker := time.NewTicker(Base / time.Duration(cfg.SpeedMultiplier))
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				cfg = Config.Get()
				currentSpeedMultiplier := cfg.SpeedMultiplier
				if currentSpeedMultiplier != speedMultiplier {
					speedMultiplier = currentSpeedMultiplier
					ticker.Reset(Base / time.Duration(speedMultiplier))
				}

				for _, id := range AllIDs() {
					status, _, ok := DefaultStore.GetState(id)
					if !ok {
						continue
					}
					roll := rand.Float64()
					switch status {
					case StatusHealthy:
						if roll < cfg.HealthyToWarningRate {
							DegradeToWarning(id)
						}
					case StatusWarning:
						if roll < cfg.WarningToCriticalRate {
							WorsenToCritical(id)
						} else if roll < cfg.WarningToCriticalRate+cfg.WarningToHealthyRate {
							RecoverToHealthy(id)
						}
					case StatusCritical:
						if roll < cfg.CriticalToWarningRate {
							StepBackToWarning(id)
						}
					default:
						slog.Error("unexpected GPU status in ticker", "gpu_id", id, "status", status)
					}
				}
			case <-ctx.Done():
				return
			}
		}
	}()
}
