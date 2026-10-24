package main

import (
	"bytes"
	"fmt"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Container represents container state and metrics.
type Container struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	Image  string `json:"image"`
	Status string `json:"status"`
	Ports  string `json:"ports"`
	State  string `json:"state"`
	CPU    string `json:"cpu,omitempty"`
	Memory string `json:"memory,omitempty"`
}

var (
	mockMu         sync.Mutex
	mockEnabled    bool
	mockContainers = []Container{
		{ID: "c1a2b3c4d5e6", Name: "m3tal-proxy", Image: "traefik:v3.0", Status: "Up 12 hours", Ports: "80->80/tcp, 443->443/tcp", State: "running", CPU: "0.12%", Memory: "32.4MiB / 16GiB"},
		{ID: "f7e8d9c0b1a2", Name: "m3tal-ai", Image: "ollama/ollama:latest", Status: "Up 12 hours", Ports: "11434->11434/tcp", State: "running", CPU: "0.05%", Memory: "512MiB / 16GiB"},
		{ID: "a3b4c5d6e7f8", Name: "m3tal-tunnel", Image: "cloudflare/cloudflared", Status: "Exited (0) 2 hours ago", Ports: "", State: "exited", CPU: "0.00%", Memory: "0B / 16GiB"},
	}
)

// SetMockMode toggles simulated data for testing or environments without a docker socket.
func SetMockMode(enabled bool) {
	mockMu.Lock()
	defer mockMu.Unlock()
	mockEnabled = enabled
}

// GetContainers returns the current list of Docker containers and their states.
func GetContainers() ([]Container, error) {
	mockMu.Lock()
	if mockEnabled {
		defer mockMu.Unlock()
		res := make([]Container, len(mockContainers))
		copy(res, mockContainers)
		return res, nil
	}
	mockMu.Unlock()

	cmd := exec.Command("docker", "ps", "-a", "--format", "{{.ID}}\t{{.Names}}\t{{.Image}}\t{{.Status}}\t{{.Ports}}\t{{.State}}")
	out, err := cmd.Output()
	if err != nil {
		// If docker daemon is unreachable, fall back to mock data so supervisor doesn't crash
		mockMu.Lock()
		defer mockMu.Unlock()
		res := make([]Container, len(mockContainers))
		copy(res, mockContainers)
		return res, nil
	}

	lines := strings.Split(strings.TrimSpace(string(out)), "\n")
	var containers []Container
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		parts := strings.Split(line, "\t")
		if len(parts) >= 6 {
			name := strings.TrimPrefix(parts[1], "/")
			containers = append(containers, Container{
				ID:     parts[0],
				Name:   name,
				Image:  parts[2],
				Status: parts[3],
				Ports:  parts[4],
				State:  strings.ToLower(parts[5]),
			})
		}
	}

	// Fetch lightweight stats for running containers with a 1.5s timeout
	if len(containers) > 0 {
		attachStats(containers)
	}

	return containers, nil
}

func attachStats(containers []Container) {
	statsCmd := exec.Command("docker", "stats", "--no-stream", "--format", "{{.ID}}\t{{.CPUPerc}}\t{{.MemUsage}}")
	statsOut, err := statsCmd.Output()
	if err != nil {
		return
	}

	statsMap := make(map[string][2]string)
	for _, l := range strings.Split(strings.TrimSpace(string(statsOut)), "\n") {
		parts := strings.Split(strings.TrimSpace(l), "\t")
		if len(parts) >= 3 {
			// ID prefix match
			statsMap[parts[0]] = [2]string{parts[1], parts[2]}
		}
	}

	for i := range containers {
		for idPrefix, stat := range statsMap {
			if strings.HasPrefix(containers[i].ID, idPrefix) || strings.HasPrefix(idPrefix, containers[i].ID) {
				containers[i].CPU = stat[0]
				containers[i].Memory = stat[1]
				break
			}
		}
	}
}

// StartContainer starts a stopped container.
func StartContainer(target string) error {
	mockMu.Lock()
	if mockEnabled {
		defer mockMu.Unlock()
		for i := range mockContainers {
			if mockContainers[i].ID == target || mockContainers[i].Name == target {
				mockContainers[i].State = "running"
				mockContainers[i].Status = "Up 1 second"
				return nil
			}
		}
		return fmt.Errorf("container %s not found", target)
	}
	mockMu.Unlock()

	out, err := exec.Command("docker", "start", target).CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to start container: %s (%v)", strings.TrimSpace(string(out)), err)
	}
	return nil
}

// StopContainer stops a running container.
func StopContainer(target string) error {
	mockMu.Lock()
	if mockEnabled {
		defer mockMu.Unlock()
		for i := range mockContainers {
			if mockContainers[i].ID == target || mockContainers[i].Name == target {
				mockContainers[i].State = "exited"
				mockContainers[i].Status = "Exited (0) Just now"
				return nil
			}
		}
		return fmt.Errorf("container %s not found", target)
	}
	mockMu.Unlock()

	out, err := exec.Command("docker", "stop", target).CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to stop container: %s (%v)", strings.TrimSpace(string(out)), err)
	}
	return nil
}

// RestartContainer restarts a container.
func RestartContainer(target string) error {
	mockMu.Lock()
	if mockEnabled {
		defer mockMu.Unlock()
		for i := range mockContainers {
			if mockContainers[i].ID == target || mockContainers[i].Name == target {
				mockContainers[i].State = "running"
				mockContainers[i].Status = "Up 1 second"
				return nil
			}
		}
		return fmt.Errorf("container %s not found", target)
	}
	mockMu.Unlock()

	out, err := exec.Command("docker", "restart", target).CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to restart container: %s (%v)", strings.TrimSpace(string(out)), err)
	}
	return nil
}

// PruneImages removes unused or dangling images.
func PruneImages() (string, error) {
	mockMu.Lock()
	if mockEnabled {
		defer mockMu.Unlock()
		return "Total reclaimed space: 142.5MB", nil
	}
	mockMu.Unlock()

	out, err := exec.Command("docker", "image", "prune", "-f").CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("failed to prune images: %s (%v)", strings.TrimSpace(string(out)), err)
	}
	lines := strings.Split(strings.TrimSpace(string(out)), "\n")
	for _, l := range lines {
		if strings.Contains(l, "reclaimed space") {
			return l, nil
		}
	}
	return "Images pruned successfully.", nil
}

// GetLogs fetches the tail logs of a container.
func GetLogs(target string, tail int) (string, error) {
	if tail <= 0 {
		tail = 50
	}
	mockMu.Lock()
	if mockEnabled {
		defer mockMu.Unlock()
		return fmt.Sprintf("[%s] Container log tail for %s\n2026-10-24 00:00:01 INFO Starting service worker\n2026-10-24 00:00:02 INFO Ready and listening", time.Now().Format(time.RFC3339), target), nil
	}
	mockMu.Unlock()

	cmd := exec.Command("docker", "logs", "--tail", strconv.Itoa(tail), target)
	var buf bytes.Buffer
	cmd.Stdout = &buf
	cmd.Stderr = &buf
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("failed to read logs: %s (%v)", strings.TrimSpace(buf.String()), err)
	}
	return buf.String(), nil
}
