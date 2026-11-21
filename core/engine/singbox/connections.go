package singbox

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"path"
	"sort"
	"strings"
	"time"
	"unicode"

	"naga.network/core/engine"
)

const maxDisplayedConnections = 100

type clashConnection struct {
	Metadata struct {
		Network         string `json:"network"`
		ProcessPath     string `json:"processPath"`
		Host            string `json:"host"`
		DestinationIP   string `json:"destinationIP"`
		DestinationPort string `json:"destinationPort"`
	} `json:"metadata"`
	Rule     string    `json:"rule"`
	Chains   []string  `json:"chains"`
	Upload   int64     `json:"upload"`
	Download int64     `json:"download"`
	Start    time.Time `json:"start"`
}

func (r *processRuntime) Connections(ctx context.Context) (engine.ConnectionSnapshot, error) {
	if r.api == nil {
		return engine.ConnectionSnapshot{}, nil
	}
	return r.api.connections(ctx)
}

func (a *runtimeAPI) connections(ctx context.Context) (engine.ConnectionSnapshot, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, a.baseURL+"/connections", nil)
	if err != nil {
		return engine.ConnectionSnapshot{}, err
	}
	req.Header.Set("Authorization", "Bearer "+a.secret)
	response, err := a.client.Do(req)
	if err != nil {
		return engine.ConnectionSnapshot{}, err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return engine.ConnectionSnapshot{}, errors.New("connection counters unavailable")
	}
	var payload struct {
		Connections []clashConnection `json:"connections"`
	}
	if err := json.NewDecoder(io.LimitReader(response.Body, 4<<20)).Decode(&payload); err != nil {
		return engine.ConnectionSnapshot{}, err
	}
	sort.SliceStable(payload.Connections, func(i, j int) bool {
		return payload.Connections[i].Start.After(payload.Connections[j].Start)
	})
	count := len(payload.Connections)
	rows := make([]engine.Connection, 0, min(count, maxDisplayedConnections))
	for _, item := range payload.Connections[:min(count, maxDisplayedConnections)] {
		rows = append(rows, a.connectionRow(item))
	}
	return engine.ConnectionSnapshot{Available: true, Total: count, Rows: rows}, nil
}

func (a *runtimeAPI) connectionRow(item clashConnection) engine.Connection {
	process := strings.ReplaceAll(item.Metadata.ProcessPath, "\\", "/")
	if suffix := strings.LastIndex(process, " ("); suffix > 0 && strings.HasSuffix(process, ")") {
		process = process[:suffix]
	}
	if process != "" {
		process = path.Base(process)
	}
	destination := item.Metadata.Host
	if destination == "" {
		destination = item.Metadata.DestinationIP
	}
	if destination != "" && item.Metadata.DestinationPort != "" {
		destination = net.JoinHostPort(destination, item.Metadata.DestinationPort)
	}
	chain := make([]string, 0, min(len(item.Chains), 12))
	for _, tag := range item.Chains[:min(len(item.Chains), 12)] {
		if metadata, ok := a.nodeMetadata[tag]; ok {
			tag = metadata.Tag
		}
		chain = append(chain, connectionLabel(tag))
	}
	return engine.Connection{
		Network: connectionLabel(item.Metadata.Network), Process: connectionLabel(process),
		Destination: connectionLabel(destination), Rule: connectionLabel(item.Rule),
		Chain: chain, Upload: max(0, item.Upload), Download: max(0, item.Download),
	}
}

func connectionLabel(value string) string {
	label := make([]rune, 0, min(len(value), 240))
	for _, ch := range value {
		if len(label) == cap(label) {
			break
		}
		if !unicode.IsControl(ch) && !unicode.Is(unicode.Bidi_Control, ch) {
			label = append(label, ch)
		}
	}
	return strings.TrimSpace(string(label))
}
