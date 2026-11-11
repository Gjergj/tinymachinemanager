package serialconsole

import (
	"context"
	"errors"
	"fmt"
	"io"
	"libvirt.org/go/libvirt"
	"sync"
	"sync/atomic"
)

const (
	OutputWindow  = 256 * 1024
	MaxInputQueue = 1024 * 1024
	frameSize     = 32 * 1024
)

type nativeStream interface {
	Recv([]byte) (int, error)
	Send([]byte) (int, error)
	Abort() error
	Free() error
	EventAddCallback(libvirt.StreamEventType, libvirt.StreamEventCallback) error
	EventUpdateCallback(libvirt.StreamEventType) error
	EventRemoveCallback() error
}

// Session serializes native operations outside libvirt's GTK-dispatched event
// callback. The callback only signals a bounded mailbox; its worker may wait for
// native I/O without blocking GTK. Credit includes queued and in-flight output.
type Session struct {
	mu                         sync.Mutex
	stream                     nativeStream
	release                    func() error
	output                     chan []byte
	done                       chan struct{}
	eventWake                  chan struct{}
	eventWorkerDone            chan struct{}
	pendingEvents              atomic.Uint32
	closed, registered, hungup bool
	credit, delivered          int
	input                      []byte
	err                        error
}

func newSession(stream nativeStream, release func() error) (*Session, error) {
	s := &Session{
		stream: stream, release: release, output: make(chan []byte, OutputWindow/frameSize),
		done: make(chan struct{}), credit: OutputWindow,
		eventWake: make(chan struct{}, 1), eventWorkerDone: make(chan struct{}),
	}
	// Native callbacks may be dispatched immediately after registration.
	s.mu.Lock()
	err := stream.EventAddCallback(libvirt.STREAM_EVENT_READABLE|libvirt.STREAM_EVENT_ERROR|libvirt.STREAM_EVENT_HANGUP, s.onEvent)
	if err == nil {
		s.registered = true
	} else {
		s.finishLocked(err)
	}
	s.mu.Unlock()
	if err != nil {
		return nil, err
	}
	go s.runEvents()
	return s, nil
}

func (s *Session) onEvent(_ *libvirt.Stream, events libvirt.StreamEventType) {
	// This runs on GTK's thread. Never take mu here: Send, Abort and even
	// callback updates can wait on libvirt's daemon transport while holding it.
	select {
	case <-s.done:
		return
	default:
	}
	s.pendingEvents.Or(uint32(events))
	select {
	case s.eventWake <- struct{}{}:
	default:
	}
}

func (s *Session) runEvents() {
	defer close(s.eventWorkerDone)
	for {
		select {
		case <-s.done:
			return
		case <-s.eventWake:
			if events := s.pendingEvents.Swap(0); events != 0 {
				s.handleEvent(libvirt.StreamEventType(events))
			}
		}
	}
}

func (s *Session) handleEvent(events libvirt.StreamEventType) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return
	}
	if events&libvirt.STREAM_EVENT_HANGUP != 0 {
		s.hungup = true
	}
	if events&(libvirt.STREAM_EVENT_READABLE|libvirt.STREAM_EVENT_HANGUP|libvirt.STREAM_EVENT_ERROR) != 0 {
		s.readLocked()
	}
	if events&libvirt.STREAM_EVENT_ERROR != 0 && !s.closed {
		s.finishLocked(errors.New("The serial console connection failed."))
		return
	}
	if events&libvirt.STREAM_EVENT_WRITABLE != 0 {
		s.writeLocked()
	}
	s.watchLocked()
}

func (s *Session) readLocked() {
	for count := 0; count < OutputWindow/frameSize && !s.closed && s.credit > 0 && len(s.output) < cap(s.output); count++ {
		b := make([]byte, min(frameSize, s.credit))
		n, err := s.stream.Recv(b)
		if err != nil {
			s.finishLocked(err)
			return
		}
		if n == -2 {
			if s.hungup {
				s.finishLocked(io.EOF)
			}
			return
		}
		if n <= 0 || n > len(b) {
			s.finishLocked(errors.New("Invalid serial stream read."))
			return
		}
		s.credit -= n
		s.output <- b[:n]
	}
}

func (s *Session) writeLocked() {
	for count := 0; count < 32 && !s.closed && len(s.input) > 0; count++ {
		n, err := s.stream.Send(s.input[:min(len(s.input), frameSize)])
		if err != nil {
			s.finishLocked(err)
			return
		}
		if n == -2 {
			return
		}
		if n <= 0 || n > len(s.input) {
			s.finishLocked(errors.New("Invalid serial stream write."))
			return
		}
		s.input = s.input[n:]
	}
	if len(s.input) == 0 {
		s.input = nil
	}
}

func (s *Session) watchLocked() {
	if s.closed {
		return
	}
	events := libvirt.STREAM_EVENT_ERROR | libvirt.STREAM_EVENT_HANGUP
	if s.credit > 0 && len(s.output) < cap(s.output) {
		events |= libvirt.STREAM_EVENT_READABLE
	}
	if len(s.input) > 0 {
		events |= libvirt.STREAM_EVENT_WRITABLE
	}
	// Do not spin on a permanent HUP while waiting for frontend credit.
	if s.hungup {
		events &^= libvirt.STREAM_EVENT_HANGUP
	}
	if err := s.stream.EventUpdateCallback(events); err != nil {
		s.finishLocked(err)
	}
}

// Read transfers an ordered chunk to the transport. Call Ack only after the
// frontend has consumed it. Buffered final data precedes EOF or a native error.
func (s *Session) Read(ctx context.Context) ([]byte, error) {
	var b []byte
	select {
	case b = <-s.output:
	default:
		select {
		case b = <-s.output:
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-s.done:
			select {
			case b = <-s.output:
			default:
				s.mu.Lock()
				err := s.err
				s.mu.Unlock()
				return nil, err
			}
		}
	}
	s.mu.Lock()
	s.delivered += len(b)
	s.watchLocked()
	s.mu.Unlock()
	return b, nil
}

func (s *Session) Ack(n int) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if n <= 0 || n > s.delivered {
		return errors.New("Invalid console output acknowledgement.")
	}
	s.delivered -= n
	s.credit += n
	if s.hungup && !s.closed {
		s.readLocked()
	}
	s.watchLocked()
	return nil
}

func (s *Session) Write(data []byte) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return io.ErrClosedPipe
	}
	if len(data) > MaxInputQueue-len(s.input) {
		return errors.New("Serial input buffer is full; disconnect and reconnect.")
	}
	s.input = append(s.input, data...)
	s.writeLocked()
	s.watchLocked()
	if s.closed {
		return s.err
	}
	return nil
}

func (s *Session) finishLocked(cause error) {
	if s.closed {
		return
	}
	s.closed = true
	s.err = cause
	if s.err == nil {
		s.err = io.EOF
	}
	if s.registered {
		if err := s.stream.EventRemoveCallback(); err != nil {
			s.err = errors.Join(s.err, err)
		}
	}
	if err := s.stream.Abort(); err != nil {
		s.err = errors.Join(s.err, err)
	}
	if err := s.stream.Free(); err != nil {
		s.err = errors.Join(s.err, err)
	}
	if err := s.release(); err != nil {
		s.err = errors.Join(s.err, fmt.Errorf("release serial connection: %w", err))
	}
	s.input = nil
	close(s.done)
}

// Close waits for native cleanup and the event worker. Call it off the GTK
// thread; an in-flight native call cannot be interrupted by a Go cancellation.
func (s *Session) Close() error {
	s.mu.Lock()
	s.finishLocked(io.EOF)
	s.mu.Unlock()
	<-s.eventWorkerDone
	return nil
}
