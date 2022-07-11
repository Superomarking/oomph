package player

import (
	"github.com/df-mc/dragonfly/server/entity/effect"
	"github.com/oomph-ac/oomph/game"
)

func (p *Player) SetEffect(id int32, eff effect.Effect) {
	p.effectMu.Lock()
	p.effects[id] = eff
	p.effectMu.Unlock()
}

func (p *Player) Effect(id int32) (effect.Effect, bool) {
	p.effectMu.Lock()
	eff, ok := p.effects[id]
	p.effectMu.Unlock()
	return eff, ok
}

func (p *Player) RemoveEffect(id int32) {
	p.effectMu.Lock()
	delete(p.effects, id)
	p.effectMu.Unlock()
}

func (p *Player) tickEffects() {
	p.effectMu.Lock()
	defer p.effectMu.Unlock()

	for i, eff := range p.effects {
		eff = eff.TickDuration()
		if eff.Duration() <= 0 {
			delete(p.effects, i)
			continue
		}

		switch eff.Type().(type) {
		case effect.JumpBoost:
			p.mInfo.JumpVelocity = game.DefaultJumpMotion + (float64(eff.Level()) / 10)
		case effect.SlowFalling:
			p.mInfo.Gravity = game.SlowFallingGravity
		case effect.Speed:
			p.mInfo.Speed += 0.02 * float64(eff.Level())
		case effect.Slowness:
			// TODO: Properly account when both speed and slowness effects are applied
			p.mInfo.Speed -= 0.015 * float64(eff.Level())
		}

		p.effects[i] = eff
	}
}
