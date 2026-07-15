package gpu

import (
	"context"
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
				// handle speed multiplier changes
				cfg = Config.Get()
				currentSpeedMultiplier := cfg.SpeedMultiplier
				if currentSpeedMultiplier != speedMultiplier {
					speedMultiplier = currentSpeedMultiplier
					ticker.Reset(Base / time.Duration(speedMultiplier))
				}

				for _, id := range AllIDs() {
					s, ok := DefaultStore.GetStatus(id)
					if !ok {
						continue
					}
					statusRoll := rand.Float64()
					switch s {
					case StatusHealthy:
						if statusRoll < cfg.HealthyToWarningRate {
							DefaultStore.SetStatus(id, StatusWarning)
						}
					case StatusWarning:
						if statusRoll < cfg.WarningToCriticalRate {
							DefaultStore.SetStatus(id, StatusCritical)
						} else if statusRoll < cfg.WarningToCriticalRate+cfg.WarningToHealthyRate {
							DefaultStore.SetStatus(id, StatusHealthy)
						}
					default:
						// In this simulation, critical will never get
						// better without manual intervention
					}
				}
			case <-ctx.Done():
				return
			}
		}
	}()
}
