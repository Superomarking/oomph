package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"github.com/RestartFU/gophig"
	"github.com/df-mc/dragonfly/server/block/cube"
	"github.com/df-mc/dragonfly/server/world"
	"github.com/justtaldevelops/oomph/player"
	"github.com/sandertv/gophertunnel/minecraft"
	"github.com/sandertv/gophertunnel/minecraft/auth"
	"github.com/sandertv/gophertunnel/minecraft/protocol/packet"
	"github.com/sirupsen/logrus"
	"golang.org/x/oauth2"
	"io/ioutil"
	"log"
	"os"
	"strings"
	"sync"
)

func main() {
	config := readConfig()
	src := tokenSource()

	p, err := minecraft.NewForeignStatusProvider(config.Connection.RemoteAddress)
	if err != nil {
		panic(err)
	}
	listener, err := minecraft.ListenConfig{
		StatusProvider: p,
	}.Listen("raknet", config.Connection.LocalAddress)
	if err != nil {
		panic(err)
	}
	defer listener.Close()
	for {
		c, err := listener.Accept()
		if err != nil {
			panic(err)
		}
		go handleConn(c.(*minecraft.Conn), listener, config, src)
	}
}

// handleConn handles a new incoming minecraft.Conn from the minecraft.Listener passed.
func handleConn(conn *minecraft.Conn, listener *minecraft.Listener, config config, src oauth2.TokenSource) {
	serverConn, err := minecraft.Dialer{
		TokenSource: src,
		ClientData:  conn.ClientData(),
	}.Dial("raknet", config.Connection.RemoteAddress)
	if err != nil {
		panic(err)
	}

	var g sync.WaitGroup
	g.Add(2)
	go func() {
		if err := conn.StartGame(serverConn.GameData()); err != nil {
			panic(err)
		}
		g.Done()
	}()
	go func() {
		if err := serverConn.DoSpawn(); err != nil {
			panic(err)
		}
		g.Done()
	}()
	g.Wait()

	lg := logrus.New()
	lg.Formatter = &logrus.TextFormatter{ForceColors: true}
	lg.Level = logrus.InfoLevel

	viewDistance := int32(8)
	p := player.NewPlayer(lg, world.Overworld, viewDistance, conn, serverConn)

	g.Add(2)
	go func() {
		defer listener.Disconnect(conn, "connection lost")
		defer serverConn.Close()
		for {
			pk, err := conn.ReadPacket()
			if err != nil {
				return
			}
			switch pk := pk.(type) {
			case *packet.Text:
				if strings.HasPrefix(pk.Message, "blockunder") {
					conn.WritePacket(&packet.Text{Message: fmt.Sprintf("%v", p.Tick())})
					conn.WritePacket(&packet.Text{Message: fmt.Sprintf("You are standing on: %T", p.Block(cube.PosFromVec3(p.Position()).Side(cube.FaceDown)))})
					continue
				}
			}
			p.Process(pk, conn)
			if err := serverConn.WritePacket(pk); err != nil {
				if disconnect, ok := errors.Unwrap(err).(minecraft.DisconnectError); ok {
					_ = listener.Disconnect(conn, disconnect.Error())
				}
				return
			}
		}
		g.Done()
	}()
	go func() {
		defer serverConn.Close()
		defer listener.Disconnect(conn, "connection lost")
		for {
			pk, err := serverConn.ReadPacket()
			if err != nil {
				if disconnect, ok := errors.Unwrap(err).(minecraft.DisconnectError); ok {
					_ = listener.Disconnect(conn, disconnect.Error())
				}
				return
			}
			p.Process(pk, serverConn)
			if err := conn.WritePacket(pk); err != nil {
				return
			}
		}
		g.Done()
	}()
	g.Wait()
	p.Close()
}

type config struct {
	Connection struct {
		LocalAddress  string
		RemoteAddress string
	}
}

func readConfig() config {
	var c config
	if _, err := os.Stat("config.toml"); os.IsNotExist(err) {
		if err := gophig.SetConfComplex("config.toml", gophig.TOMLMarshaler{}, c, 0777); err != nil {
			log.Fatalf("error creating config: %v", err)
		}
	}
	if err := gophig.GetConfComplex("config.toml", gophig.TOMLMarshaler{}, &c); err != nil {
		log.Fatalf("error reading config: %v", err)
	}
	if c.Connection.LocalAddress == "" {
		c.Connection.LocalAddress = "0.0.0.0:19132"
	}
	if err := gophig.SetConfComplex("config.toml", gophig.TOMLMarshaler{}, c, 0777); err != nil {
		log.Fatalf("error writing config file: %v", err)
	}
	return c
}

// tokenSource returns a token source for using with a gophertunnel client. It either reads it from the
// token.tok file if cached or requests logging in with a device code.
func tokenSource() oauth2.TokenSource {
	check := func(err error) {
		if err != nil {
			panic(err)
		}
	}
	token := new(oauth2.Token)
	tokenData, err := ioutil.ReadFile("token.tok")
	if err == nil {
		_ = json.Unmarshal(tokenData, token)
	} else {
		token, err = auth.RequestLiveToken()
		check(err)
	}
	src := auth.RefreshTokenSource(token)
	_, err = src.Token()
	if err != nil {
		// The cached refresh token expired and can no longer be used to obtain a new token. We require the
		// user to log in again and use that token instead.
		token, err = auth.RequestLiveToken()
		check(err)
		src = auth.RefreshTokenSource(token)
	}
	tok, _ := src.Token()
	b, _ := json.Marshal(tok)
	_ = ioutil.WriteFile("token.tok", b, 0644)
	return src
}
