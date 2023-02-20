package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"

	"time"

	"github.com/gorilla/websocket"

	"github.com/pion/rtcp"
	"github.com/pion/webrtc"
	"github.com/pion/webrtc/pkg/media"
	"github.com/pion/webrtc/pkg/media/ivfwriter"
)

func webRTCHandle(w http.ResponseWriter, r *http.Request) {
	conn, err := newConnHandler(w, r)
	if err != nil {
		fmt.Fprint(w, "Unable to connect: ", err)
	}

	conn.startUserMedia()
	conn.sendIceCandidates()
	conn.sendOffer()
	conn.startListener()
}

var upgrader = websocket.Upgrader{
	ReadBufferSize:  512,
	WriteBufferSize: 512,
}

var defaultIceServers = webrtc.Configuration{
	ICEServers: []webrtc.ICEServer{
		{
			URLs: []string{"stun:stun.l.google.com:19302"},
		},
	},
}

type connHandler struct {
	socket *websocket.Conn
	peerConnection *webrtc.PeerConnection
}

func newConnHandler(w http.ResponseWriter, r *http.Request) (connHandler, error) {
	conn := connHandler{}

	// Upgrade http to websocket
	ws, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		return conn, err
	}

	// Create new webrtc peer connection obj
	// Note: connection to client won't be formed until negotiate() is called
	pc, err := newPeerConnection()
	if err != nil {
		return conn, err
	}

	conn.socket = ws
	conn.peerConnection = pc

	return conn, nil
}

func newPeerConnection() (*webrtc.PeerConnection, error) {
	m := webrtc.MediaEngine{}
	m.RegisterCodec(webrtc.NewRTPVP8Codec(webrtc.DefaultPayloadTypeVP8, 90000))
	api := webrtc.NewAPI(webrtc.WithMediaEngine(m))

	peerConnection, err := api.NewPeerConnection(defaultIceServers)
	if err != nil {
		return nil, err
	}

	if _, err = peerConnection.AddTransceiverFromKind(webrtc.RTPCodecTypeVideo); err != nil {
		return nil, err
	}

	return peerConnection, nil
}

func (conn *connHandler) send(message interface{}) {
	payload, err := json.Marshal(message)
	if err != nil {
		log.Println(err)
	}

	err = conn.socket.WriteMessage(websocket.TextMessage, payload)
	if err != nil {
		log.Println("message not sent")
	}
}

func (conn *connHandler) startUserMedia() {
	ivfFile, err := ivfwriter.New("output.ivf")
	if err != nil {
		log.Println(err)
	}

	conn.peerConnection.OnTrack(func(track *webrtc.Track, receiver *webrtc.RTPReceiver) {
		// Send heartbeat - copied from Pion example code
		go func() {
			ticker := time.NewTicker(time.Second * 3)
			for range ticker.C {
				errSend := conn.peerConnection.WriteRTCP([]rtcp.Packet{&rtcp.PictureLossIndication{MediaSSRC: track.SSRC()}})
				if errSend != nil {
					log.Println(errSend)
				}
			}
		}()

		codec := track.Codec()

		if codec.Name == webrtc.VP8 {
			saveToDisk(ivfFile, track)
		}
	})
}

func (conn *connHandler) sendIceCandidates() {
	conn.peerConnection.OnICECandidate(func(c *webrtc.ICECandidate) {
		if c == nil {
			return
		}
		conn.send(c.ToJSON())
	})
}

func (conn *connHandler) sendOffer() {
	offer, err := conn.peerConnection.CreateOffer(nil)
	if err != nil {
		log.Println(err)
	}

	if err = conn.peerConnection.SetLocalDescription(offer); err != nil {
		log.Println(err)
	}

	conn.send(offer)
}

func (conn *connHandler) startListener() {
	var renegotiate string
	var candidate webrtc.ICECandidateInit
	var sessDesc webrtc.SessionDescription

	for {
		_, p, err := conn.socket.ReadMessage()
		if err != nil {
			conn.peerConnection.Close()
			return
		}

		if err = json.Unmarshal(p, &candidate); err != nil {
			conn.peerConnection.AddICECandidate(candidate)
			candidate = webrtc.ICECandidateInit{}
			
		} else if err = json.Unmarshal(p, &sessDesc); sessDesc.Type == webrtc.SDPTypeAnswer {
			conn.peerConnection.SetRemoteDescription(sessDesc)
			sessDesc = webrtc.SessionDescription{}

		} else if err = json.Unmarshal(p, &renegotiate); renegotiate == "renegotiate" {
			conn.sendOffer()
			renegotiate = ""

		}
	}
}

func saveToDisk(i media.Writer, track *webrtc.Track) {
	defer func() {
		if err := i.Close(); err != nil {
			log.Println(err)
		}
	}()

	for {
		rtpPacket, err := track.ReadRTP()
		if err != nil {
			log.Println(err)
		}
		fmt.Println(rtpPacket.Payload)
		if err := i.WriteRTP(rtpPacket); err != nil {
			log.Println(err)
		}
	}
}
