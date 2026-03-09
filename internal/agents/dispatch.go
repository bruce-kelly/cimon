package agents

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"
)

// DispatchedAgent tracks a locally-spawned Claude Code agent.
type DispatchedAgent struct {
	ID         string
	Repo       string
	Task       string
	PID        int
	StartedAt  time.Time
	Status     string // running, completed, failed, killed
	ExitCode   *int
	PRNumber   *int
	OutputPath string
	cmd        *exec.Cmd
}

// Dispatcher manages locally-spawned Claude Code agents.
type Dispatcher struct {
	agents     map[string]*DispatchedAgent
	maxConc    int
	maxLife    time.Duration
	captureOut bool
	outputDir  string
	mu         sync.Mutex
}

func NewDispatcher(maxConcurrent int, maxLifetimeSec int, captureOutput bool) *Dispatcher {
	outputDir := filepath.Join(os.Getenv("HOME"), ".local", "share", "cimon", "agent-output")
	os.MkdirAll(outputDir, 0o755)
	return &Dispatcher{
		agents:     make(map[string]*DispatchedAgent),
		maxConc:    maxConcurrent,
		maxLife:    time.Duration(maxLifetimeSec) * time.Second,
		captureOut: captureOutput,
		outputDir:  outputDir,
	}
}

// Dispatch spawns a new Claude Code agent. Returns the agent ID or error.
func (d *Dispatcher) Dispatch(repo, task string) (string, error) {
	d.mu.Lock()
	defer d.mu.Unlock()

	// Check concurrency limit
	running := 0
	for _, a := range d.agents {
		if a.Status == "running" {
			running++
		}
	}
	if running >= d.maxConc {
		return "", fmt.Errorf("max concurrent agents (%d) reached", d.maxConc)
	}

	id := fmt.Sprintf("agent-%d", time.Now().UnixNano())
	cmd := exec.Command("claude", "-p", task)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}

	agent := &DispatchedAgent{
		ID:        id,
		Repo:      repo,
		Task:      task,
		StartedAt: time.Now(),
		Status:    "running",
		cmd:       cmd,
	}

	if d.captureOut {
		agent.OutputPath = filepath.Join(d.outputDir, id+".log")
		f, err := os.Create(agent.OutputPath)
		if err != nil {
			return "", fmt.Errorf("creating output file: %w", err)
		}
		cmd.Stdout = f
		cmd.Stderr = f
	} else {
		cmd.Stdout = os.NewFile(0, os.DevNull)
		cmd.Stderr = os.NewFile(0, os.DevNull)
	}

	if err := cmd.Start(); err != nil {
		return "", fmt.Errorf("starting agent: %w", err)
	}
	agent.PID = cmd.Process.Pid
	d.agents[id] = agent

	// Background goroutine to wait for exit
	go func() {
		err := cmd.Wait()
		d.mu.Lock()
		defer d.mu.Unlock()
		if err != nil {
			agent.Status = "failed"
			if exitErr, ok := err.(*exec.ExitError); ok {
				code := exitErr.ExitCode()
				agent.ExitCode = &code
			}
		} else {
			agent.Status = "completed"
			code := 0
			agent.ExitCode = &code
		}
	}()

	return id, nil
}

// GetAgent returns a dispatched agent by ID.
func (d *Dispatcher) GetAgent(id string) *DispatchedAgent {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.agents[id]
}

// RunningAgents returns all agents with status "running".
func (d *Dispatcher) RunningAgents() []*DispatchedAgent {
	d.mu.Lock()
	defer d.mu.Unlock()
	var result []*DispatchedAgent
	for _, a := range d.agents {
		if a.Status == "running" {
			result = append(result, a)
		}
	}
	return result
}

// AllAgents returns all tracked agents.
func (d *Dispatcher) AllAgents() []*DispatchedAgent {
	d.mu.Lock()
	defer d.mu.Unlock()
	var result []*DispatchedAgent
	for _, a := range d.agents {
		result = append(result, a)
	}
	return result
}

// GetOutput reads the last N lines of agent output.
func (d *Dispatcher) GetOutput(id string, tailLines int) (string, error) {
	d.mu.Lock()
	agent := d.agents[id]
	d.mu.Unlock()
	if agent == nil {
		return "", fmt.Errorf("agent not found: %s", id)
	}
	if agent.OutputPath == "" {
		return "", nil
	}
	data, err := os.ReadFile(agent.OutputPath)
	if err != nil {
		return "", err
	}
	lines := strings.Split(string(data), "\n")
	if len(lines) > tailLines {
		lines = lines[len(lines)-tailLines:]
	}
	return strings.Join(lines, "\n"), nil
}

// CheckAll polls running agents for exit, kills those past max lifetime.
func (d *Dispatcher) CheckAll() {
	d.mu.Lock()
	defer d.mu.Unlock()
	for _, agent := range d.agents {
		if agent.Status != "running" {
			continue
		}
		// Kill if past max lifetime
		if d.maxLife > 0 && time.Since(agent.StartedAt) > d.maxLife {
			syscall.Kill(-agent.PID, syscall.SIGTERM)
			agent.Status = "killed"
		}
	}
}

// Shutdown terminates all running agents.
func (d *Dispatcher) Shutdown() {
	d.mu.Lock()
	defer d.mu.Unlock()
	for _, agent := range d.agents {
		if agent.Status == "running" {
			syscall.Kill(-agent.PID, syscall.SIGTERM)
			agent.Status = "killed"
		}
	}
}
