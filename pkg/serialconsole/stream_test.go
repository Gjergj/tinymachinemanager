package serialconsole

import (
	"bytes"
	"context"
	"errors"
	"io"
	"libvirt.org/go/libvirt"
	"sync"
	"testing"
	"time"
)

type fakeStream struct {
	input                   []byte
	output                  bytes.Buffer
	wouldBlock              bool
	eof                     bool
	callback                libvirt.StreamEventCallback
	watches                 libvirt.StreamEventType
	aborted, freed, removed int
}

func (f *fakeStream) Recv(p []byte) (int, error) {
	if len(f.input) > 0 {
		n := copy(p, f.input)
		f.input = f.input[n:]
		return n, nil
	}
	if f.eof {
		return 0, io.EOF
	}
	return -2, nil
}
func (f *fakeStream) Send(p []byte) (int, error) {
	if f.wouldBlock {
		return -2, nil
	}
	n := min(2, len(p))
	return f.output.Write(p[:n])
}
func (f *fakeStream) Abort() error { f.aborted++; return nil }
func (f *fakeStream) Free() error  { f.freed++; return nil }
func (f *fakeStream) EventAddCallback(e libvirt.StreamEventType, c libvirt.StreamEventCallback) error {
	f.watches = e
	f.callback = c
	return nil
}
func (f *fakeStream) EventUpdateCallback(e libvirt.StreamEventType) error { f.watches = e; return nil }
func (f *fakeStream) EventRemoveCallback() error                          { f.removed++; return nil }
func (f *fakeStream) fire(e libvirt.StreamEventType)                      { f.callback(nil, e) }

// Native operations run asynchronously after callback dispatch. Inspect fake
// state under the same serialization lock used by the session, never by timing
// an unsynchronized read against its event worker.
func waitStream(t *testing.T, s *Session, ready func() bool) {
	t.Helper()
	deadline := time.Now().Add(time.Second)
	for {
		s.mu.Lock()
		ok := ready()
		s.mu.Unlock()
		if ok {
			return
		}
		if time.Now().After(deadline) {
			t.Fatal("timed out waiting for stream processing")
		}
		time.Sleep(time.Millisecond)
	}
}

func TestStreamPreservesPartialWritesAndWouldBlock(t *testing.T) {
	f := &fakeStream{wouldBlock: true}
	s, err := newSession(f, func() error { return nil })
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if err = s.Write([]byte("abc\x1b[Aé")); err != nil {
		t.Fatal(err)
	}
	if f.watches&libvirt.STREAM_EVENT_WRITABLE == 0 {
		t.Fatal("pending input not watched")
	}
	s.mu.Lock()
	f.wouldBlock = false
	s.mu.Unlock()
	f.fire(libvirt.STREAM_EVENT_WRITABLE)
	waitStream(t, s, func() bool { return f.output.String() == "abc\x1b[Aé" && f.watches&libvirt.STREAM_EVENT_WRITABLE == 0 })
	s.mu.Lock()
	defer s.mu.Unlock()
	if f.output.String() != "abc\x1b[Aé" {
		t.Fatalf("input changed: %q", f.output.String())
	}
	if f.watches&libvirt.STREAM_EVENT_WRITABLE != 0 {
		t.Fatal("idle writable watch would spin")
	}
}

func TestStreamDrainsHangupBeforeEOFAndCleansOnce(t *testing.T) {
	f := &fakeStream{input: []byte("\x1b[31mRocky café"), eof: true}
	released := 0
	s, err := newSession(f, func() error { released++; return nil })
	if err != nil {
		t.Fatal(err)
	}
	f.fire(libvirt.STREAM_EVENT_READABLE | libvirt.STREAM_EVENT_HANGUP)
	data, err := s.Read(context.Background())
	if err != nil || string(data) != "\x1b[31mRocky café" {
		t.Fatalf("lost final output: %q %v", data, err)
	}
	_, err = s.Read(context.Background())
	if !errors.Is(err, io.EOF) {
		t.Fatalf("expected EOF: %v", err)
	}
	s.Close()
	s.Close()
	if released != 1 || f.aborted != 1 || f.freed != 1 || f.removed != 1 {
		t.Fatalf("cleanup repeated: %+v released=%d", f, released)
	}
}

func TestStreamBackpressureAndInvalidAcknowledgement(t *testing.T) {
	f := &fakeStream{input: bytes.Repeat([]byte("x"), OutputWindow+100), eof: true}
	s, err := newSession(f, func() error { return nil })
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	f.fire(libvirt.STREAM_EVENT_READABLE | libvirt.STREAM_EVENT_HANGUP)
	waitStream(t, s, func() bool { return len(f.input) == 100 })
	if err = s.Ack(1); err == nil {
		t.Fatal("acknowledged unsent bytes")
	}
	total := 0
	for total < OutputWindow+100 {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		b, err := s.Read(ctx)
		cancel()
		if err != nil {
			t.Fatal(err)
		}
		total += len(b)
		if err = s.Ack(len(b)); err != nil {
			t.Fatal(err)
		}
	}
	if total != OutputWindow+100 {
		t.Fatal("output lost")
	}
}

func TestStreamRejectsInputOverflow(t *testing.T) {
	f := &fakeStream{wouldBlock: true}
	s, err := newSession(f, func() error { return nil })
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if err = s.Write(make([]byte, MaxInputQueue+1)); err == nil {
		t.Fatal("unbounded input accepted")
	}
}

// The daemon transport can block in Send even with STREAM_NONBLOCK. Its event
// callback must still return so GTK can dispatch the rest of the application.
type blockedSendStream struct {
	fakeStream
	started, resume chan struct{}
	once            sync.Once
}

func (f *blockedSendStream) Send(p []byte) (int, error) {
	f.once.Do(func() { close(f.started) })
	<-f.resume
	return f.fakeStream.Send(p)
}

func TestStreamCallbackReturnsWhileSendIsBlocked(t *testing.T) {
	f := &blockedSendStream{started: make(chan struct{}), resume: make(chan struct{})}
	s, err := newSession(f, func() error { return nil })
	if err != nil {
		t.Fatal(err)
	}
	written := make(chan error, 1)
	go func() { written <- s.Write([]byte("abc\x1b[Aé")) }()
	<-f.started
	returned := make(chan struct{})
	go func() {
		f.fire(libvirt.STREAM_EVENT_READABLE | libvirt.STREAM_EVENT_WRITABLE)
		close(returned)
	}()
	defer func() {
		close(f.resume)
		if err := <-written; err != nil {
			t.Error(err)
		}
		<-returned
		s.Close()
		if f.output.String() != "abc\x1b[Aé" {
			t.Errorf("input changed: %q", f.output.String())
		}
	}()
	select {
	case <-returned:
	case <-time.After(time.Second):
		t.Fatal("GTK callback waited for the blocked native send")
	}
}

type blockedAbortStream struct {
	fakeStream
	started, resume chan struct{}
}

func (f *blockedAbortStream) Abort() error {
	close(f.started)
	<-f.resume
	return f.fakeStream.Abort()
}

func TestStreamCallbackReturnsWhileCleanupIsBlocked(t *testing.T) {
	f := &blockedAbortStream{fakeStream: fakeStream{eof: true}, started: make(chan struct{}), resume: make(chan struct{})}
	released := 0
	s, err := newSession(f, func() error { released++; return nil })
	if err != nil {
		t.Fatal(err)
	}
	returned := make(chan struct{})
	go func() { f.fire(libvirt.STREAM_EVENT_HANGUP); close(returned) }()
	<-f.started
	defer func() {
		close(f.resume)
		<-returned
		s.Close()
		if f.aborted != 1 || f.freed != 1 || f.removed != 1 || released != 1 {
			t.Errorf("cleanup counts: abort=%d free=%d remove=%d release=%d", f.aborted, f.freed, f.removed, released)
		}
	}()
	select {
	case <-returned:
	case <-time.After(time.Second):
		t.Fatal("GTK callback waited for native cleanup")
	}
}

func TestStreamCoalescesEventsWithoutLosingFinalOutput(t *testing.T) {
	f := &blockedSendStream{
		fakeStream: fakeStream{input: []byte("final \x1b[31mcafé\x1b[0m")},
		started:    make(chan struct{}), resume: make(chan struct{}),
	}
	s, err := newSession(f, func() error { return nil })
	if err != nil {
		t.Fatal(err)
	}
	written := make(chan error, 1)
	go func() { written <- s.Write([]byte("input")) }()
	<-f.started
	returned := make(chan struct{})
	go func() {
		for i := 0; i < 10000; i++ {
			f.fire(libvirt.STREAM_EVENT_READABLE)
		}
		f.fire(libvirt.STREAM_EVENT_HANGUP)
		// An overwrite instead of an OR would lose the hangup at this point.
		f.fire(libvirt.STREAM_EVENT_WRITABLE)
		close(returned)
	}()
	var resumeOnce sync.Once
	resume := func() { resumeOnce.Do(func() { close(f.resume) }) }
	defer func() { resume(); <-returned; s.Close() }()
	select {
	case <-returned:
	case <-time.After(time.Second):
		t.Fatal("callback notifications blocked behind the busy session")
	}
	resume()
	if err = <-written; err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	data, err := s.Read(ctx)
	if err != nil || string(data) != "final \x1b[31mcafé\x1b[0m" {
		t.Fatalf("final output: %q, %v", data, err)
	}
	if _, err = s.Read(ctx); !errors.Is(err, io.EOF) {
		t.Fatalf("coalesced hangup was lost: %v", err)
	}
}

type registrationStream struct {
	fakeStream
	err error
}

func (f *registrationStream) EventAddCallback(events libvirt.StreamEventType, cb libvirt.StreamEventCallback) error {
	f.fakeStream.EventAddCallback(events, cb)
	// Registration can dispatch immediately, before the event worker starts.
	cb(nil, libvirt.STREAM_EVENT_READABLE)
	return f.err
}

func TestStreamRetainsEventsDuringRegistration(t *testing.T) {
	f := &registrationStream{fakeStream: fakeStream{input: []byte("initial output"), eof: true}}
	s, err := newSession(f, func() error { return nil })
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	data, err := s.Read(ctx)
	if err != nil || string(data) != "initial output" {
		t.Fatalf("initial output: %q, %v", data, err)
	}
}

func TestStreamRegistrationFailureReleasesResources(t *testing.T) {
	failure := errors.New("could not register callback")
	f := &registrationStream{err: failure}
	released := 0
	s, err := newSession(f, func() error { released++; return nil })
	if s != nil || !errors.Is(err, failure) {
		t.Fatalf("registration result: %v, %v", s, err)
	}
	if f.aborted != 1 || f.freed != 1 || f.removed != 0 || released != 1 {
		t.Fatalf("registration cleanup: %+v; releases=%d", f, released)
	}
}

func TestStreamCallbacksRacingWithConcurrentCloseCleanUpOnce(t *testing.T) {
	f := &fakeStream{}
	released := 0
	s, err := newSession(f, func() error { released++; return nil })
	if err != nil {
		t.Fatal(err)
	}
	var callers sync.WaitGroup
	start := make(chan struct{})
	for i := 0; i < 8; i++ {
		callers.Go(func() {
			<-start
			for n := 0; n < 100; n++ {
				f.fire(libvirt.STREAM_EVENT_READABLE | libvirt.STREAM_EVENT_WRITABLE)
			}
			s.Close()
			f.fire(libvirt.STREAM_EVENT_ERROR | libvirt.STREAM_EVENT_HANGUP)
		})
	}
	close(start)
	callers.Wait()
	if f.aborted != 1 || f.freed != 1 || f.removed != 1 || released != 1 {
		t.Fatalf("concurrent cleanup: %+v; releases=%d", f, released)
	}
	if err = s.Write([]byte("late input")); !errors.Is(err, io.ErrClosedPipe) {
		t.Fatalf("closed stream accepted input: %v", err)
	}
}
