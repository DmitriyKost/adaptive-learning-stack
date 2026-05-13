package service

import (
	"math"
	"time"
)

type EbbinghausConfig struct {
	MinHalfLifeDays float64
	MaxHalfLifeDays float64
}

type Ebbinghaus struct {
	cfg EbbinghausConfig
}

func NewEbbinghaus(cfg EbbinghausConfig) *Ebbinghaus {
	return &Ebbinghaus{cfg: cfg}
}

func (e *Ebbinghaus) Retention(now time.Time, updatedAt time.Time, mastery float64) float64 {
	if updatedAt.IsZero() {
		return 0
	}
	days := now.Sub(updatedAt).Hours() / 24
	if days < 0 {
		days = 0
	}
	halfLifeDays := e.cfg.MinHalfLifeDays + clamp(mastery, 0, 1)*(e.cfg.MaxHalfLifeDays-e.cfg.MinHalfLifeDays)
	if halfLifeDays <= 0 {
		halfLifeDays = 1
	}
	retention := math.Pow(0.5, days/halfLifeDays)
	return clamp(retention, 0, 1)
}

func (e *Ebbinghaus) ForgettingPriority(now time.Time, updatedAt time.Time, mastery float64) float64 {
	return 1 - e.Retention(now, updatedAt, mastery)
}

func (e *Ebbinghaus) EffectiveMastery(now time.Time, updatedAt time.Time, mastery float64) float64 {
	return clamp(mastery*e.Retention(now, updatedAt, mastery), 0, 1)
}

func clamp(v, min, max float64) float64 {
	if v < min {
		return min
	}
	if v > max {
		return max
	}
	return v
}
