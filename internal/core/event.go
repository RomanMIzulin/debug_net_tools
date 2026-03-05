package core

import (
	"time"

	"github.com/google/uuid"
)

type Direction int8

const (
	ClientToServer Direction = iota
	ServerToClient
)

type Frame struct {
	Opcode uint8 // 1=text, 2=binary, 8=close, 9=ping, 10=pong
	Payload []byte
	Direction Direction
	Timestamp time.Time
}

func NewFrame(opcode uint8, payload []byte, direction Direction) *Frame {
	return &Frame{
		Opcode: opcode,
		Payload: payload,
		Direction: direction,
	}
}

type SessionState int

type Message struct {
	Frames []Frame
	ID uuid.UUID
	Text string
	Bytes []byte // want to join all bytes from all frames?
}

const (
	StateCreated SessionState = iota
	StateConnecting
	StateActive // proxying and recording frames
	StatePaused // for inspect edit & send
	StateClosing // close frame is sent, waiting for response
	StateClosed
	StateError // without graceful shutdown
)

type Session struct {
	frames []Frame
	ID uuid.UUID
	Target string
	State SessionState
	CreatedAt time.Time
	ClosedAt time.Time
	UpdatedAt time.Time
	CloseCode uint16 // 1000 normal, 1006 abnormal
}


func NewSession(target string) *Session {
	return &Session{
		frames: []Frame{},
		ID: uuid.New(),
		Target: target,
		State: StateCreated,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
}
