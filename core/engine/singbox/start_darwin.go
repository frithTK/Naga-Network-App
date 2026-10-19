//go:build darwin && !ios

package singbox

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"time"
)

const darwinConfigLimit = 8 * 1024 * 1024

func defaultReadyTimeout() time.Duration        { return 15 * time.Second }
func restrictConfigAccess(string, string) error { return nil }
func RunTunHost(string) error                   { return errTunHostUnsupported }

func startPlatformProcess(ctx context.Context, req processRequest) (startedProcess, error) {
	data, err := readDarwinRuleSet(req.ConfigPath)
	if err != nil {
		return nil, err
	}
	var doc map[string]any
	if err := json.Unmarshal(data, &doc); err != nil {
		return nil, err
	}
	if !hasTUNInbound(doc) {
		return startCommandProcess(ctx, req)
	}
	if len(data) > darwinConfigLimit {
		return nil, errors.New("Конфигурация VPN превышает 8 МБ")
	}
	// Rule-set files are read/decompiled by the user process. The helper only
	// receives inline rules and cannot be asked to open user-selected paths.
	if err := inlineDarwinRuleSets(ctx, doc, req.Binary); err != nil {
		return nil, err
	}
	data, err = json.Marshal(doc)
	if err != nil {
		return nil, err
	}
	if len(data) > darwinConfigLimit {
		return nil, errors.New("Правила VPN превышают 8 МБ")
	}
	self, err := os.Executable()
	if err != nil {
		return nil, err
	}
	bridge := filepath.Join(filepath.Dir(self), "NagaVPNBridge")
	cmd := exec.Command(bridge)
	cmd.Env = []string{"PATH=/usr/bin:/bin:/usr/sbin:/sbin"}
	input, err := cmd.StdinPipe()
	if err != nil {
		return nil, err
	}
	output, err := cmd.StdoutPipe()
	if err != nil {
		_ = input.Close()
		return nil, err
	}
	cmd.Stderr = req.Stderr
	if err := cmd.Start(); err != nil {
		_ = input.Close()
		return nil, fmt.Errorf("Запуск службы VPN: %w", err)
	}
	p := &darwinProcess{cmd: cmd, input: input, done: make(chan struct{}), ready: make(chan error, 1)}
	go p.read(output)
	// Cancellation must also break a blocked config write, before readiness.
	go func() {
		select {
		case <-ctx.Done():
			_ = p.SignalStop()
		case <-p.done:
		}
	}()
	startup := time.AfterFunc(20*time.Second, func() { _ = p.SignalStop() })
	defer startup.Stop()
	if _, err := fmt.Fprintf(input, "%s\n", data); err != nil {
		_ = p.SignalStop()
		return nil, err
	}
	select {
	case err := <-p.ready:
		if err != nil {
			_ = p.SignalStop()
			return nil, err
		}
	case <-ctx.Done():
		_ = p.SignalStop()
		return nil, ctx.Err()
	case <-time.After(20 * time.Second):
		_ = p.SignalStop()
		return nil, errors.New("Служба VPN не ответила. Разрешите Naga в настройках фоновых элементов")
	}
	return p, nil
}

type darwinProcess struct {
	cmd   *exec.Cmd
	input io.WriteCloser
	stop  sync.Once
	done  chan struct{}
	ready chan error
	err   error
}

func (p *darwinProcess) read(output io.Reader) {
	scanner := bufio.NewScanner(output)
	scanner.Buffer(make([]byte, 4096), 16384)
	gotReady := false
	cleaned := false
	for scanner.Scan() {
		var event struct {
			Type            string `json:"type"`
			Message         string `json:"message"`
			ProtocolVersion int    `json:"protocol_version"`
			Cleanup         bool   `json:"cleanup"`
		}
		if json.Unmarshal(scanner.Bytes(), &event) != nil {
			continue
		}
		switch event.Type {
		case "ready":
			if event.ProtocolVersion != 1 {
				p.err = errors.New("Несовместимая версия VPN bridge")
				_ = p.SignalStop()
				continue
			}
			if !gotReady {
				gotReady = true
				p.ready <- nil
			}
		case "error":
			p.err = fmt.Errorf("Служба VPN: %s", event.Message)
			if !gotReady {
				gotReady = true
				p.ready <- p.err
			}
		case "stopped":
			cleaned = event.Cleanup
		}
	}
	if err := scanner.Err(); err != nil && p.err == nil {
		p.err = fmt.Errorf("Чтение ответа службы VPN: %w", err)
		_ = p.SignalStop()
	}
	if err := p.cmd.Wait(); err != nil && p.err == nil {
		p.err = fmt.Errorf("Соединение со службой VPN завершилось: %w", err)
	}
	if gotReady && !cleaned && p.err == nil {
		p.err = errors.New("Служба VPN не подтвердила восстановление сети")
	}
	if !gotReady {
		p.ready <- errors.New("Служба VPN недоступна. Повторите подключение в приложении")
	}
	close(p.done)
}

func (p *darwinProcess) Wait() error { <-p.done; return p.err }
func (p *darwinProcess) SignalStop() error {
	p.stop.Do(func() {
		_ = p.input.Close()
		go func() {
			timer := time.NewTimer(30 * time.Second)
			defer timer.Stop()
			select {
			case <-p.done:
			case <-timer.C:
				_ = p.cmd.Process.Kill() // XPC disconnect triggers the helper's owned cleanup.
			}
		}()
	})
	return nil
}

// Closing the authenticated bridge makes the helper stop only its owned engine.
func (p *darwinProcess) Kill() error { return p.SignalStop() }

func inlineDarwinRuleSets(ctx context.Context, doc map[string]any, binary string) error {
	route, _ := doc["route"].(map[string]any)
	sets, _ := route["rule_set"].([]any)
	for _, raw := range sets {
		set, ok := raw.(map[string]any)
		if !ok || set["type"] != "local" {
			continue
		}
		path, _ := set["path"].(string)
		var data []byte
		var err error
		if set["format"] == "binary" {
			tmp, e := os.CreateTemp("", "naga-rules-*.json")
			if e != nil {
				return e
			}
			output := tmp.Name()
			_ = tmp.Close()
			defer os.Remove(output)
			checkCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
			cmd := exec.CommandContext(checkCtx, binary, "rule-set", "decompile", "--output", output, path)
			err = cmd.Run()
			cancel()
			if err == nil {
				data, err = readDarwinRuleSet(output)
			}
		} else {
			data, err = readDarwinRuleSet(path)
		}
		if err != nil {
			return fmt.Errorf("Подготовка правил macOS: %w", err)
		}
		var rules struct {
			Rules []any `json:"rules"`
		}
		if err := json.Unmarshal(data, &rules); err != nil {
			return err
		}
		set["type"] = "inline"
		set["rules"] = rules.Rules
		delete(set, "path")
		delete(set, "format")
	}
	return nil
}

func readDarwinRuleSet(path string) ([]byte, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	data, err := io.ReadAll(io.LimitReader(f, darwinConfigLimit+1))
	if len(data) > darwinConfigLimit {
		return nil, errors.New("rule-set exceeds 8 MiB")
	}
	return data, err
}
