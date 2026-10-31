package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"time"

	"agentdesk/internal/desk"
)

func serverOptions(c desk.Config, o options) options {
	if !o.listenExplicit && c.Server.Listen != "" {
		o.listen = c.Server.Listen
		o.listenExplicit = true
	} else if !o.listenExplicit {
		// Older configurations may only record the selected port in Endpoint.
		// Keep NIC autodetection enabled while reusing that project's port.
		endpoint, err := url.Parse(c.Endpoint)
		host, port, listenErr := net.SplitHostPort(o.listen)
		if err == nil && listenErr == nil && port != "0" && endpoint.Port() != "" {
			o.listen = net.JoinHostPort(host, endpoint.Port())
		}
	}
	if !o.publicURLExplicit {
		o.publicURL = c.Server.PublicURL
	}
	if !o.telegramExplicit {
		o.telegram = c.Server.Telegram
	}
	if o.telegramConfig == "" {
		o.telegramConfig = c.Server.TelegramConfig
	}
	return o
}

func probeServer(ctx context.Context, dir string) (json.RawMessage, error) {
	c, err := desk.LoadConfig(dir)
	if err != nil {
		return nil, err
	}
	client := apiClient{config: c, http: &http.Client{Timeout: 800 * time.Millisecond}}
	v, err := client.request(ctx, "GET", "/status", nil, "")
	if err != nil {
		return nil, err
	}
	var status struct {
		ProjectID string `json:"project_id"`
		PID       int    `json:"pid"`
	}
	if json.Unmarshal(v, &status) != nil || status.ProjectID != c.ProjectID || status.PID <= 0 {
		return nil, errors.New("project service identity mismatch")
	}
	return v, nil
}

// startServer returns only after an authenticated project identity probe.
func startServer(ctx context.Context, dir string, c desk.Config, o options) (int, error) {
	if status, err := probeServer(ctx, dir); err == nil {
		return output(map[string]any{"started": false, "already_running": true, "service": status})
	}
	o = serverOptions(c, o)
	executable, err := os.Executable()
	if err != nil {
		return 1, err
	}
	args := []string{"serve", "--data-dir", dir, "--telegram=" + strconv.FormatBool(o.telegram)}
	if o.listenExplicit {
		args = append(args, "--listen", o.listen)
	}
	if o.publicURL != "" {
		args = append(args, "--public-url", o.publicURL)
	}
	if o.telegramConfig != "" && o.telegram {
		args = append(args, "--telegram-config", o.telegramConfig)
	}
	if o.open {
		args = append(args, "--open")
	}
	logPath := filepath.Join(dir, "server.log")
	logFile, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0600)
	if err != nil {
		return 1, err
	}
	cmd := exec.Command(executable, args...)
	cmd.Stdin = nil
	cmd.Stdout = logFile
	cmd.Stderr = logFile
	detach(cmd)
	err = cmd.Start()
	logFile.Close()
	if err != nil {
		return 1, errors.New("cannot launch detached project service")
	}
	if err = cmd.Process.Release(); err != nil {
		return 1, errors.New("cannot release detached project service handle")
	}
	waitCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	for {
		if status, err := probeServer(waitCtx, dir); err == nil {
			return output(map[string]any{"started": true, "already_running": false, "service": status})
		}
		select {
		case <-waitCtx.Done():
			return 1, fmt.Errorf("server did not become ready; inspect %s", logPath)
		case <-time.After(100 * time.Millisecond):
		}
	}
}
