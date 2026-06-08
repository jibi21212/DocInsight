package main

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"time"
)

// sidecarProcess is the spawned Python embedding service.
type sidecarProcess struct {
	cmd *exec.Cmd
	url string
}

// startSidecar locates the embedding sidecar, picks a free localhost port, spawns
// uvicorn from the sidecar's virtualenv, and waits until /health responds.
func startSidecar(ctx context.Context) (*sidecarProcess, error) {
	dir, err := locateSidecarDir()
	if err != nil {
		return nil, err
	}
	py := venvPython(dir)
	if py == "" {
		return nil, fmt.Errorf("python venv not found in %s — run setup.ps1", dir)
	}
	port, err := freePort()
	if err != nil {
		return nil, err
	}

	cmd := exec.CommandContext(ctx, py, "-m", "uvicorn", "main:app", "--host", "127.0.0.1", "--port", fmt.Sprint(port))
	cmd.Dir = dir
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("failed to start sidecar: %w", err)
	}

	url := fmt.Sprintf("http://127.0.0.1:%d", port)
	if err := waitHealthy(url, 60*time.Second); err != nil {
		_ = cmd.Process.Kill()
		return nil, err
	}
	return &sidecarProcess{cmd: cmd, url: url}, nil
}

// locateSidecarDir finds the directory containing the sidecar's main.py, trying
// (in order) an env override, locations next to the executable, and the dev tree.
func locateSidecarDir() (string, error) {
	var candidates []string
	if env := os.Getenv("EMBEDDING_SIDECAR_DIR"); env != "" {
		candidates = append(candidates, env)
	}
	if exe, err := os.Executable(); err == nil {
		d := filepath.Dir(exe)
		candidates = append(candidates,
			filepath.Join(d, "embedding-sidecar"),
			filepath.Join(d, "backend", "embedding-sidecar"),
		)
	}
	if wd, err := os.Getwd(); err == nil {
		candidates = append(candidates,
			filepath.Join(wd, "backend", "embedding-sidecar"),
			filepath.Join(wd, "embedding-sidecar"),
		)
	}
	for _, c := range candidates {
		if _, err := os.Stat(filepath.Join(c, "main.py")); err == nil {
			return c, nil
		}
	}
	return "", fmt.Errorf("embedding-sidecar/main.py not found (looked in %d locations) — run setup.ps1", len(candidates))
}

// venvPython returns the venv interpreter path for the current OS, or "".
func venvPython(dir string) string {
	for _, rel := range []string{
		filepath.Join(".venv", "Scripts", "python.exe"), // Windows
		filepath.Join(".venv", "bin", "python"),         // POSIX
	} {
		p := filepath.Join(dir, rel)
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}
	return ""
}

// freePort asks the OS for an unused localhost TCP port.
func freePort() (int, error) {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return 0, err
	}
	defer l.Close()
	return l.Addr().(*net.TCPAddr).Port, nil
}

// waitHealthy polls <url>/health until it returns 200 or the timeout elapses
// (first start includes model load, so allow generous time).
func waitHealthy(url string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	client := &http.Client{Timeout: 2 * time.Second}
	for time.Now().Before(deadline) {
		if resp, err := client.Get(url + "/health"); err == nil {
			resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				return nil
			}
		}
		time.Sleep(500 * time.Millisecond)
	}
	return fmt.Errorf("sidecar did not become healthy within %s", timeout)
}

// stop terminates the sidecar process.
func (s *sidecarProcess) stop() {
	if s != nil && s.cmd != nil && s.cmd.Process != nil {
		_ = s.cmd.Process.Kill()
	}
}
