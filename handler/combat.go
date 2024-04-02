package handler

import (
	"github.com/chewxy/math32"
	"github.com/ethaniccc/float32-cube/cube/trace"
	"github.com/go-gl/mathgl/mgl32"
	"github.com/oomph-ac/oomph/entity"
	"github.com/oomph-ac/oomph/game"
	"github.com/oomph-ac/oomph/player"
	"github.com/oomph-ac/oomph/utils"
	"github.com/sandertv/gophertunnel/minecraft/protocol"
	"github.com/sandertv/gophertunnel/minecraft/protocol/packet"
)

const HandlerIDCombat = "oomph:combat"

const (
	CombatPhaseNone byte = iota
	CombatPhaseTransaction
	CombatPhaseTicked
)

type CombatHandler struct {
	Phase          byte
	TargetedEntity *entity.Entity

	InterpolationStep float32
	AttackOffset      float32

	StartAttackPos mgl32.Vec3
	EndAttackPos   mgl32.Vec3

	StartEntityPos mgl32.Vec3
	EndEntityPos   mgl32.Vec3

	ClosestRawDistance float32
	RaycastResults     []float32

	LastSwingTick int64

	Clicking      bool
	Clicks        []int64
	ClickDelay    int64
	LastClickTick int64
	CPS           int
}

func NewCombatHandler() *CombatHandler {
	return &CombatHandler{
		InterpolationStep: 1 / 10.0,
	}
}

func (h *CombatHandler) ID() string {
	return HandlerIDCombat
}

func (h *CombatHandler) HandleClientPacket(pk packet.Packet, p *player.Player) bool {
	switch pk := pk.(type) {
	case *packet.InventoryTransaction:
		if h.Phase != CombatPhaseNone {
			return true
		}

		dat, ok := pk.TransactionData.(*protocol.UseItemOnEntityTransactionData)
		if !ok {
			return true
		}

		if dat.ActionType != protocol.UseItemOnEntityActionAttack {
			return true
		}

		h.click(p)

		entity := p.Handler(HandlerIDEntities).(*EntitiesHandler).Find(dat.TargetEntityRuntimeID)
		if entity == nil {
			return true
		}

		if entity.TicksSinceTeleport <= 20 {
			return true
		}

		mDat := p.Handler(HandlerIDMovement).(*MovementHandler)
		if mDat.TicksSinceTeleport <= 20 {
			return true
		}

		h.AttackOffset = float32(1.62)
		if mDat.Sneaking {
			h.AttackOffset = 1.54
		}

		h.Phase = CombatPhaseTransaction
		h.TargetedEntity = entity

		h.StartAttackPos = mDat.PrevClientPosition.Add(mgl32.Vec3{0, h.AttackOffset})
		h.EndAttackPos = mDat.ClientPosition.Add(mgl32.Vec3{0, h.AttackOffset})

		h.StartEntityPos = entity.PrevPosition
		h.EndEntityPos = entity.Position

		// Calculate the closest raw point from the attack positions to the entity's bounding box.
		bb1 := entity.Box(entity.PrevPosition).Grow(0.1)
		bb2 := entity.Box(entity.Position).Grow(0.1)

		point1 := game.ClosestPointToBBox(h.StartAttackPos, bb1)
		point2 := game.ClosestPointToBBox(h.EndAttackPos, bb1)
		point3 := game.ClosestPointToBBox(h.StartAttackPos, bb2)
		point4 := game.ClosestPointToBBox(h.EndAttackPos, bb2)

		close1 := math32.Min(
			point1.Sub(h.StartAttackPos).Len(),
			point2.Sub(h.EndAttackPos).Len(),
		)
		close2 := math32.Min(
			point3.Sub(h.StartAttackPos).Len(),
			point4.Sub(h.EndAttackPos).Len(),
		)

		h.ClosestRawDistance = math32.Min(close1, close2)
	case *packet.PlayerAuthInput:
		if p.Version >= player.GameVersion1_20_10 && utils.HasFlag(pk.InputData, packet.InputFlagMissedSwing) {
			h.click(p)
		}

		if h.Phase != CombatPhaseTransaction {
			return true
		}
		h.Phase = CombatPhaseTicked

		// The entity may have already been removed before we are able to do anything with it.
		if h.TargetedEntity == nil {
			h.Phase = CombatPhaseNone
			return true
		}

		// Ignore touch input, as they are able to interact with entities without actually looking at them.
		if pk.InputMode == packet.InputModeTouch {
			return true
		}
		h.calculatePointingResults(p)
	case *packet.Animate:
		h.LastSwingTick = p.ClientFrame
	case *packet.LevelSoundEvent:
		if p.Version < player.GameVersion1_20_10 && pk.SoundType == packet.SoundEventAttackNoDamage {
			h.click(p)
		}
	}

	return true
}

func (h *CombatHandler) HandleServerPacket(pk packet.Packet, p *player.Player) bool {
	return true
}

func (*CombatHandler) OnTick(p *player.Player) {
}

func (h *CombatHandler) Defer() {
	if h.Phase == CombatPhaseTicked {
		h.Phase = CombatPhaseNone
	}

	h.Clicking = false
}

func (h *CombatHandler) calculatePointingResults(p *player.Player) {
	mDat := p.Handler(HandlerIDMovement).(*MovementHandler)
	attackPosDelta := h.EndAttackPos.Sub(h.StartAttackPos)
	entityPosDelta := h.EndEntityPos.Sub(h.StartEntityPos)

	startRotation := mDat.PrevRotation
	endRotation := mDat.Rotation
	rotationDelta := endRotation.Sub(startRotation)
	if rotationDelta.Len() >= 180 {
		return
	}

	altEntityStartPos := h.TargetedEntity.PrevPosition
	altEntityEndPos := h.TargetedEntity.Position
	altEntityPosDelta := altEntityEndPos.Sub(altEntityStartPos)

	/* altStartAttackPos := mDat.PrevClientPosition.Add(mgl32.Vec3{0, h.AttackOffset})
	altEndAttackPos := mDat.ClientPosition.Add(mgl32.Vec3{0, h.AttackOffset})
	altAttackPosDelta := altEndAttackPos.Sub(altStartAttackPos) */

	h.RaycastResults = make([]float32, 0, 20)
	for partialTicks := float32(0); partialTicks <= 1; partialTicks += h.InterpolationStep {
		attackPos := h.StartAttackPos.Add(attackPosDelta.Mul(partialTicks))
		entityPos := h.StartEntityPos.Add(entityPosDelta.Mul(partialTicks))
		bb := h.TargetedEntity.Box(entityPos).Grow(0.1)

		rotation := startRotation.Add(rotationDelta.Mul(partialTicks))
		directionVec := game.DirectionVector(rotation.Z(), rotation.X()).Mul(14)

		result, ok := trace.BBoxIntercept(bb, attackPos, attackPos.Add(directionVec))
		if ok {
			h.RaycastResults = append(h.RaycastResults, attackPos.Sub(result.Position()).Len())
		}

		// An extra raycast is ran here with the current entity position, as the client may have ticked
		// the entity to a new position while the frame logic was running (where attacks are done).
		entityPos = altEntityStartPos.Add(altEntityPosDelta.Mul(partialTicks))
		bb = h.TargetedEntity.Box(entityPos).Grow(0.1)
		result, ok = trace.BBoxIntercept(bb, attackPos, attackPos.Add(directionVec))
		if ok {
			h.RaycastResults = append(h.RaycastResults, attackPos.Sub(result.Position()).Len())
		}
	}

}

// Click adds a click to the player's click history.
func (h *CombatHandler) click(p *player.Player) {
	currentTick := p.ClientFrame

	h.Clicking = true
	if len(h.Clicks) > 0 {
		h.ClickDelay = (currentTick - h.LastClickTick) * 50
	} else {
		h.ClickDelay = 0
	}
	h.Clicks = append(h.Clicks, currentTick)
	var clicks []int64
	for _, clickTick := range h.Clicks {
		if currentTick-clickTick <= 20 {
			clicks = append(clicks, clickTick)
		}
	}
	h.LastClickTick = currentTick
	h.Clicks = clicks
	h.CPS = len(h.Clicks)
}
