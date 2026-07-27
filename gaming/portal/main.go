// Command portal supervises the games a user has installed.
//
// It follows the pattern every other service in this stack uses, because the
// dashboard cannot start anything: there is no docker socket and no exec, so a
// container runs continuously and the process inside it idles until a file in
// the control directory says otherwise. dcrlnd waits for .account, brclientd
// waits for tls.cert, and this waits for a game to appear in installedGames.
//
// What differs is trust. Every other service in the stack is trusted and mounts
// /app-data, where dcrwallet's files and brclientd's client certificate live.
// A game is the first component that is not trusted, and that certificate is
// the credential for Bison Relay's RPC - a game able to read it would talk to
// the relay directly and the bridge in front of it would be decoration. So this
// container mounts neither, sits on a network with no route off the host, and
// reaches exactly one thing: the dashboard.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"sort"
	"sync"
	"syscall"
	"time"
)

const (
	// pollInterval matches the other supervisors in the stack. The dashboard
	// has no way to signal, so noticing a change means looking for it.
	pollInterval = 3 * time.Second

	// stopGrace is how long a game gets to exit on its own before it is
	// killed. A game may be mid-hand with money escrowed, so it is worth
	// waiting for it to shut down in an orderly way.
	stopGrace = 10 * time.Second

	// basePort is where games start listening, one port each.
	basePort = 8790

	// maxGames bounds the port range. It is deliberately small: this is a
	// sandbox for the games a person added, not a hosting platform.
	maxGames = 32
)

type settings struct {
	Enabled        bool              `json:"enabled"`
	InstalledGames []string          `json:"installedGames"`
	GameTokens     map[string]string `json:"gameTokens"`
	Rev            int               `json:"rev"`
}

// state is what the dashboard reads to know what is actually running. It is the
// only way the answer travels back, so "installed" and "running" stay separate:
// a game can be installed and crashed, and the dashboard should say so rather
// than claim it is ready.
type state struct {
	Rev     int            `json:"rev"`
	Running map[string]int `json:"running"` // game id -> pid
	// Ports is where each running game listens, so the dashboard can reach
	// it. The sandbox has no route out, so this file is the only way the
	// answer travels, and a game the dashboard cannot address is one a user
	// cannot accept an invitation into.
	Ports   map[string]int `json:"ports"`
	Missing []string       `json:"missing"` // installed, but no binary present
	Updated int64          `json:"updated"`
}

type portal struct {
	controlPath string
	statePath   string
	gamesDir    string
	bridgeURL   string

	mu      sync.Mutex
	running map[string]*exec.Cmd
	// ports remembers which port each game was given, so a game that is
	// restarted after crashing comes back where the dashboard last saw it.
	ports map[string]int
}

// portFor assigns a game its listening port, keeping the one it already has.
// The caller holds p.mu.
//
// The range is small and private to the sandbox: nothing outside it can reach
// these, because the only network the container is on has the dashboard at one
// end and nothing at the other.
func (p *portal) portFor(id string) int {
	if port, ok := p.ports[id]; ok {
		return port
	}
	taken := make(map[int]bool, len(p.ports))
	for _, port := range p.ports {
		taken[port] = true
	}
	for port := basePort; port < basePort+maxGames; port++ {
		if !taken[port] {
			p.ports[id] = port
			return port
		}
	}
	return 0
}

func main() {
	var (
		control  = flag.String("control", "/control/gaming.json", "path to the gaming policy the dashboard writes")
		stateOut = flag.String("state", "/data/control-state.json", "path this portal reports through")
		games    = flag.String("games", "/data/games", "directory holding installed game binaries")
		bridge   = flag.String("bridge", "http://dashboard:8080/gaming", "the dashboard's gaming tunnel")
	)
	flag.Parse()

	p := &portal{
		controlPath: *control,
		statePath:   *stateOut,
		gamesDir:    *games,
		bridgeURL:   *bridge,
		running:     make(map[string]*exec.Cmd),
		ports:       make(map[string]int),
	}

	if err := os.MkdirAll(p.gamesDir, 0o700); err != nil {
		log.Fatalf("portal: games directory: %v", err)
	}
	if err := os.MkdirAll(filepath.Dir(p.statePath), 0o700); err != nil {
		log.Fatalf("portal: state directory: %v", err)
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	log.Printf("portal: watching %s", p.controlPath)
	p.run(ctx)

	// Games hold escrowed funds, so they are given the chance to shut down
	// in an orderly way rather than being killed with the container.
	p.stopAll()
	log.Printf("portal: stopped")
}

func (p *portal) run(ctx context.Context) {
	ticker := time.NewTicker(pollInterval)
	defer ticker.Stop()

	lastRev := -1
	for {
		s, err := p.readSettings()
		if err != nil {
			// A missing or unreadable policy means nothing is installed,
			// which is the safe reading: it stops games rather than
			// leaving them running on a policy nobody can see.
			s = settings{}
		}

		// A rev bump forces a reconcile even when the content looks
		// identical, which is why the dashboard maintains one - a policy
		// can be rewritten to the same values and still need games
		// restarted.
		if s.Rev != lastRev {
			p.reconcile(s)
			lastRev = s.Rev
		} else {
			p.reapDead(s)
		}
		p.writeState(s)

		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

// wanted reports the games that should be running.
//
// A disabled section wants none. That is the same invariant the dashboard
// enforces when it refuses to enable a section with no account bound: a game
// with nothing to stake should not be playing.
func wanted(s settings) map[string]string {
	out := map[string]string{}
	if !s.Enabled {
		return out
	}
	for _, id := range s.InstalledGames {
		if tok := s.GameTokens[id]; tok != "" {
			out[id] = tok
			continue
		}
		// No token means no identity, and the tunnel would refuse it
		// anyway. Starting it would only produce a game that cannot
		// speak.
		log.Printf("portal: %s is installed but has no token; not starting it", id)
	}
	return out
}

// reconcile starts what should run and stops what should not.
func (p *portal) reconcile(s settings) {
	want := wanted(s)

	p.mu.Lock()
	defer p.mu.Unlock()

	for id, cmd := range p.running {
		if _, keep := want[id]; keep && cmd.Process != nil {
			continue
		}
		p.stopLocked(id)
	}

	for id, token := range want {
		if cmd, ok := p.running[id]; ok && cmd.Process != nil {
			continue
		}
		p.startLocked(id, token)
	}
}

// reapDead restarts anything that exited on its own.
func (p *portal) reapDead(s settings) {
	want := wanted(s)

	p.mu.Lock()
	defer p.mu.Unlock()

	for id, token := range want {
		cmd, ok := p.running[id]
		if ok && cmd.ProcessState == nil {
			continue // still running
		}
		if ok {
			log.Printf("portal: %s exited (%v); restarting", id, cmd.ProcessState)
			delete(p.running, id)
		}
		p.startLocked(id, token)
	}
}

func (p *portal) binaryFor(id string) string { return filepath.Join(p.gamesDir, id) }

// startLocked launches a game. The caller holds p.mu.
func (p *portal) startLocked(id, token string) {
	bin := p.binaryFor(id)
	if _, err := os.Stat(bin); err != nil {
		// Installed in policy but not present on disk. Fetch it through
		// the host, which is the only route out of the sandbox, and
		// verify it before it is ever written under a name this portal
		// will execute.
		if err := p.install(id, token); err != nil {
			log.Printf("portal: %v", err)
			return
		}
	}

	// The token goes on the command line the same way brclientd's
	// supervisor passes its cert paths. It is the game's identity: the
	// dashboard resolves it to decide which game is calling, so a game
	// never states which game it is.
	port := p.portFor(id)
	if port == 0 {
		log.Printf("portal: no port left for %s; not starting it", id)
		return
	}

	cmd := exec.Command(bin,
		"--bridge="+p.bridgeURL,
		"--token="+token,
		// Where the dashboard reaches this game to drive it - accepting
		// an invitation, and later the game's own interface. It cannot
		// be loopback: the dashboard is a different container.
		fmt.Sprintf("--listen=:%d", port),
	)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	// A game gets no environment. Anything it needs arrives as a flag, so
	// nothing leaks in from the container that was not meant for it.
	cmd.Env = []string{}

	if err := cmd.Start(); err != nil {
		log.Printf("portal: could not start %s: %v", id, err)
		return
	}
	p.running[id] = cmd
	log.Printf("portal: started %s (pid %d) on port %d", id, cmd.Process.Pid, port)

	// Reap it so a crashed game does not linger as a zombie between polls.
	go func() { _ = cmd.Wait() }()
}

// stopLocked ends a game. The caller holds p.mu.
func (p *portal) stopLocked(id string) {
	cmd, ok := p.running[id]
	delete(p.running, id)
	if !ok || cmd.Process == nil {
		return
	}

	_ = cmd.Process.Signal(syscall.SIGTERM)
	done := make(chan struct{})
	go func() { _, _ = cmd.Process.Wait(); close(done) }()
	select {
	case <-done:
	case <-time.After(stopGrace):
		log.Printf("portal: %s did not stop; killing it", id)
		_ = cmd.Process.Kill()
	}

	// Uninstalling revokes rather than hides: the binary goes too, so a
	// game cannot be left on disk to be started by anything else.
	if err := os.Remove(p.binaryFor(id)); err != nil && !os.IsNotExist(err) {
		log.Printf("portal: could not remove %s: %v", id, err)
	}
	log.Printf("portal: stopped %s", id)
}

func (p *portal) stopAll() {
	p.mu.Lock()
	defer p.mu.Unlock()
	for id, cmd := range p.running {
		if cmd.Process != nil {
			_ = cmd.Process.Signal(syscall.SIGTERM)
		}
		delete(p.running, id)
	}
}

func (p *portal) readSettings() (settings, error) {
	blob, err := os.ReadFile(p.controlPath)
	if err != nil {
		return settings{}, err
	}
	var s settings
	if err := json.Unmarshal(blob, &s); err != nil {
		return settings{}, fmt.Errorf("parse %s: %w", p.controlPath, err)
	}
	return s, nil
}

// writeState reports what is running, atomically, the way every supervisor in
// this stack reports.
func (p *portal) writeState(s settings) {
	p.mu.Lock()
	st := state{Rev: s.Rev, Running: map[string]int{}, Ports: map[string]int{}, Updated: time.Now().Unix()}
	for id, cmd := range p.running {
		if cmd.Process != nil && cmd.ProcessState == nil {
			st.Running[id] = cmd.Process.Pid
			if port, ok := p.ports[id]; ok {
				st.Ports[id] = port
			}
		}
	}
	p.mu.Unlock()

	for id := range wanted(s) {
		if _, up := st.Running[id]; up {
			continue
		}
		if _, err := os.Stat(p.binaryFor(id)); err != nil {
			st.Missing = append(st.Missing, id)
		}
	}
	sort.Strings(st.Missing)

	blob, err := json.MarshalIndent(st, "", "  ")
	if err != nil {
		return
	}
	tmp := p.statePath + ".tmp"
	if err := os.WriteFile(tmp, blob, 0o600); err != nil {
		log.Printf("portal: write state: %v", err)
		return
	}
	if err := os.Rename(tmp, p.statePath); err != nil {
		log.Printf("portal: replace state: %v", err)
	}
}
