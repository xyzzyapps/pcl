package shell

import (
	"context"
	"fmt"
	"io"
	"os/exec"
	"sync"
	"sync/atomic"
	"time"
)

const (
	// FibStart is the first foreground slice when the model does not set timeout.
	FibStart = 1 * time.Second
	// FibCap is the largest silence slice between Fibonacci steps.
	FibCap = 34 * time.Second
	// MaxBlock is the longest foreground wait the model may request.
	MaxBlock = 10 * time.Minute
)

// Session is a tracked shell process that may outlive a single tool call.
type Session struct {
	ID  string
	Pid int
	Cmd string

	mu      sync.Mutex
	stdout  notifyBuf
	stderr  notifyBuf
	done    chan struct{}
	waitErr error
	running bool
	proc    *exec.Cmd
}

func (s *Session) snapshot() (out, errStr string, running bool, waitErr error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.stdout.String(), s.stderr.String(), s.running, s.waitErr
}

func (s *Session) Output() string {
	out, errStr, _, _ := s.snapshot()
	return mergeOut(out, errStr)
}

func (s *Session) Running() bool {
	_, _, running, _ := s.snapshot()
	return running
}

func (s *Session) Kill() error {
	s.mu.Lock()
	cmd := s.proc
	s.mu.Unlock()
	if cmd == nil {
		return nil
	}
	killCmd(cmd)
	return nil
}

type sessionStore struct {
	mu   sync.Mutex
	next atomic.Int64
	all  map[string]*Session
}

var sessions = &sessionStore{all: map[string]*Session{}}

func (st *sessionStore) put(s *Session) {
	st.mu.Lock()
	st.all[s.ID] = s
	st.mu.Unlock()
}

func GetSession(id string) (*Session, bool) {
	sessions.mu.Lock()
	defer sessions.mu.Unlock()
	s, ok := sessions.all[id]
	return s, ok
}

func ListSessions() []*Session {
	sessions.mu.Lock()
	defer sessions.mu.Unlock()
	out := make([]*Session, 0, len(sessions.all))
	for _, s := range sessions.all {
		out = append(out, s)
	}
	return out
}

func KillSession(id string) error {
	s, ok := GetSession(id)
	if !ok {
		return fmt.Errorf("no shell session %s", id)
	}
	return s.Kill()
}

type notifyBuf struct {
	mu   sync.Mutex
	b    []byte
	ping chan struct{}
	live io.Writer
}

func (n *notifyBuf) Write(p []byte) (int, error) {
	n.mu.Lock()
	n.b = append(n.b, p...)
	n.mu.Unlock()
	if n.live != nil {
		_, _ = n.live.Write(p)
	}
	if n.ping != nil {
		select {
		case n.ping <- struct{}{}:
		default:
		}
	}
	return len(p), nil
}

func (n *notifyBuf) String() string {
	n.mu.Lock()
	defer n.mu.Unlock()
	return string(n.b)
}

// AwaitOpts controls foreground waiting for a shell tool call.
type AwaitOpts struct {
	Timeout    time.Duration // 0 = DefaultBlock; <0 = wait until exit or cancel
	Background bool
	Live       io.Writer
}

// SessionResult is returned to the agent after a shell tool call.
type SessionResult struct {
	ID       string
	Pid      int
	Stdout   string
	Stderr   string
	Err      error
	Running  bool
	ExitCode int
}

func (r SessionResult) Combined() string {
	return mergeOut(r.Stdout, r.Stderr)
}

func mergeOut(out, errStr string) string {
	switch {
	case out == "":
		return errStr
	case errStr == "":
		return out
	default:
		return out + "\n" + errStr
	}
}

// StartAndAwait runs a script the way coding agents do: wait in the
// foreground (timeout grows while output is flowing), then keep the
// process in the background instead of killing it. ctx cancel (Ctrl+C)
// still kills a process that is being waited on.
func StartAndAwait(ctx context.Context, script string, opts AwaitOpts) SessionResult {
	if ctx == nil {
		ctx = context.Background()
	}

	bin, prefix, err := FindPOSIXShell()
	if err != nil {
		return SessionResult{Err: err}
	}

	id := fmt.Sprintf("sh_%d", sessions.next.Add(1))
	ping := make(chan struct{}, 1)
	s := &Session{
		ID:      id,
		Cmd:     script,
		done:    make(chan struct{}),
		running: true,
	}
	s.stdout.ping = ping
	s.stderr.ping = ping
	s.stdout.live = opts.Live
	s.stderr.live = opts.Live

	args := append(append([]string{}, prefix...), "-c", script)
	cmd := exec.Command(bin, args...)
	applySysProcAttr(cmd)
	cmd.Stdout = &s.stdout
	cmd.Stderr = &s.stderr
	s.proc = cmd

	if err := cmd.Start(); err != nil {
		return SessionResult{Err: err}
	}
	if cmd.Process != nil {
		s.Pid = cmd.Process.Pid
	}

	go func() {
		waitErr := cmd.Wait()
		s.mu.Lock()
		s.waitErr = waitErr
		s.running = false
		s.mu.Unlock()
		close(s.done)
	}()

	if opts.Background {
		sessions.put(s)
		out, errStr, _, _ := s.snapshot()
		return SessionResult{
			ID:      s.ID,
			Pid:     s.Pid,
			Stdout:  out,
			Stderr:  errStr,
			Running: true,
		}
	}

	return awaitForeground(ctx, s, ping, opts.Timeout)
}

func awaitForeground(ctx context.Context, s *Session, ping <-chan struct{}, timeout time.Duration) SessionResult {
	if timeout < 0 {
		return waitUntilDone(ctx, s)
	}
	if timeout > 0 {
		if timeout > MaxBlock {
			timeout = MaxBlock
		}
		return waitUntil(ctx, s, time.Now().Add(timeout))
	}
	return waitFibonacci(ctx, s, ping)
}

func waitUntilDone(ctx context.Context, s *Session) SessionResult {
	select {
	case <-s.done:
		return finishedResult(s)
	case <-ctx.Done():
		return killedResult(ctx, s)
	}
}

func waitUntil(ctx context.Context, s *Session, deadline time.Time) SessionResult {
	remain := time.Until(deadline)
	if remain < 0 {
		remain = 0
	}
	timer := time.NewTimer(remain)
	defer timer.Stop()
	select {
	case <-s.done:
		return finishedResult(s)
	case <-ctx.Done():
		return killedResult(ctx, s)
	case <-timer.C:
		return backgroundResult(s)
	}
}

func waitFibonacci(ctx context.Context, s *Session, ping <-chan struct{}) SessionResult {
	fib1, fib2 := FibStart, FibStart
	started := time.Now()
	for {
		if time.Since(started) >= MaxBlock {
			return backgroundResult(s)
		}
		slice := fib1
		if slice > FibCap {
			slice = FibCap
		}
		if remain := MaxBlock - time.Since(started); slice > remain {
			slice = remain
		}
		gotOut, res, done := waitSlice(ctx, s, ping, slice)
		if done {
			return res
		}
		if !gotOut {
			return backgroundResult(s)
		}
		fib1, fib2 = fib2, fib1+fib2
	}
}

func waitSlice(ctx context.Context, s *Session, ping <-chan struct{}, slice time.Duration) (gotOut bool, res SessionResult, finished bool) {
	deadline := time.Now().Add(slice)
	for {
		remain := time.Until(deadline)
		if remain <= 0 {
			select {
			case <-s.done:
				return gotOut, finishedResult(s), true
			default:
				return gotOut, SessionResult{}, false
			}
		}
		timer := time.NewTimer(remain)
		select {
		case <-s.done:
			timer.Stop()
			return gotOut, finishedResult(s), true
		case <-ctx.Done():
			timer.Stop()
			return gotOut, killedResult(ctx, s), true
		case <-ping:
			timer.Stop()
			gotOut = true
		case <-timer.C:
			select {
			case <-s.done:
				return gotOut, finishedResult(s), true
			default:
				return gotOut, SessionResult{}, false
			}
		}
	}
}

func killedResult(ctx context.Context, s *Session) SessionResult {
	_ = s.Kill()
	<-s.done
	out, errStr, _, waitErr := s.snapshot()
	if waitErr == nil {
		waitErr = ctx.Err()
	}
	return SessionResult{ID: s.ID, Pid: s.Pid, Stdout: out, Stderr: errStr, Err: waitErr}
}

func backgroundResult(s *Session) SessionResult {
	select {
	case <-s.done:
		return finishedResult(s)
	default:
	}
	sessions.put(s)
	out, errStr, _, _ := s.snapshot()
	return SessionResult{
		ID:      s.ID,
		Pid:     s.Pid,
		Stdout:  out,
		Stderr:  errStr,
		Running: true,
	}
}

func finishedResult(s *Session) SessionResult {
	out, errStr, _, waitErr := s.snapshot()
	return SessionResult{
		ID:     s.ID,
		Pid:    s.Pid,
		Stdout: out,
		Stderr: errStr,
		Err:    waitErr,
	}
}
