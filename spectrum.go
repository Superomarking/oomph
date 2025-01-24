package oomph

import (
	"os"
	"sync/atomic"
	"time"

	"github.com/cooldogedev/spectrum/session"
	"github.com/oomph-ac/oomph/player"
	"github.com/oomph-ac/oomph/player/component"
	"github.com/oomph-ac/oomph/player/detection"
	"github.com/sandertv/gophertunnel/minecraft"
	"github.com/sandertv/gophertunnel/minecraft/protocol"
	"github.com/sandertv/gophertunnel/minecraft/protocol/login"
	"github.com/sandertv/gophertunnel/minecraft/protocol/packet"
	"github.com/sirupsen/logrus"
)

var _ session.Processor = &Processor{}

type Processor struct {
	session.NopProcessor

	identity login.IdentityData
	registry *session.Registry
	pl       atomic.Pointer[player.Player]

	dbgTransfer bool
}

func NewProcessor(
	s *session.Session,
	registry *session.Registry,
	listener *minecraft.Listener,
	log *logrus.Logger,
) *Processor {
	pl := player.New(log, player.MonitoringState{
		IsReplay:    false,
		IsRecording: false,
		CurrentTime: time.Now(),
	}, listener)
	pl.SetConn(s.Client())

	component.Register(pl)
	detection.Register(pl)

	go pl.StartTicking()
	p := &Processor{identity: s.Client().IdentityData(), registry: registry}
	p.pl.Store(pl)
	return p
}

func (p *Processor) ProcessStartGame(ctx *session.Context, gd *minecraft.GameData) {
	gd.PlayerMovementSettings.MovementType = protocol.PlayerMovementModeServerWithRewind
	gd.PlayerMovementSettings.RewindHistorySize = 40
}

func (p *Processor) ProcessServer(ctx *session.Context, pk packet.Packet) {
	if pl := p.pl.Load(); pl != nil {
		pl.HandleServerPacket(pk)
	}
}

func (p *Processor) ProcessClient(ctx *session.Context, pk packet.Packet) {
	if os.Getenv("DBG") != "" {
		if txt, ok := pk.(*packet.Text); ok && txt.Message == "transferme" {
			if !p.dbgTransfer {
				p.registry.GetSession(p.identity.XUID).Transfer("127.0.0.1:20002")
				p.dbgTransfer = true
			} else {
				p.registry.GetSession(p.identity.XUID).Transfer("127.0.0.1:20000")
				p.dbgTransfer = false
			}
		}
	}

	if pl := p.pl.Load(); pl != nil && pl.HandleClientPacket(pk) {
		ctx.Cancel()
	}
}

func (p *Processor) ProcessPreTransfer(*session.Context, *string, *string) {
	if pl := p.pl.Load(); pl != nil {
		pl.PauseProcessing()
		pl.ACKs().Invalidate()
		pl.World.PurgeChunks()
	}
}

func (p *Processor) ProcessPostTransfer(_ *session.Context, _ *string, _ *string) {
	if s, pl := p.registry.GetSession(p.identity.XUID), p.pl.Load(); s != nil && pl != nil {
		pl.SetServerConn(s.Server())
		pl.ResumeProcessing()
	}
}

func (p *Processor) ProcessTransferFailure(_ *session.Context, origin *string, target *string) {
	if s, pl := p.registry.GetSession(p.identity.XUID), p.pl.Load(); s != nil && pl != nil {
		pl.ResumeProcessing()
	}
}

func (p *Processor) ProcessDisconnection(_ *session.Context) {
	if pl := p.pl.Load(); pl != nil {
		_ = pl.Close()
		p.pl.Store(nil)
	}
}

func (p *Processor) Player() *player.Player {
	return p.pl.Load()
}
