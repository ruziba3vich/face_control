// wssniff opens a WebSocket connection to the FaceGate camera, prints every
// incoming frame, and pipes anything you type on stdin out as a frame. Use it
// to watch what the web UI sends when you click "register a person", or to
// poke the device manually with handcrafted commands.
//
// Usage:
//   go run ./cmd/wssniff
//   go run ./cmd/wssniff -url ws://192.0.0.22:8000/ -user admin -pass admin
//
// Once connected, type a JSON command on stdin and press enter, e.g.:
//   {"cmd":"get params"}
//   {"cmd":"get help"}
//
// Heartbeat (ping) is sent automatically every 20s.
package main

import (
	"bufio"
	"context"
	"encoding/base64"
	"encoding/json"
	"flag"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/coder/websocket"
)

type logEntry struct {
	TS        string          `json:"ts"`
	Direction string          `json:"dir"` // "recv" | "send" | "info" | "error"
	Type      string          `json:"type,omitempty"` // "text" | "binary"
	Bytes     int             `json:"bytes,omitempty"`
	Text      string          `json:"text,omitempty"`
	JSON      json.RawMessage `json:"json,omitempty"` // populated when Text parses as JSON
	Hex       string          `json:"hex_prefix,omitempty"` // first 64 bytes of binary frames, hex
	Note      string          `json:"note,omitempty"`
}

func main() {
	wsURL := flag.String("url", "ws://192.0.0.22:8000/", "WebSocket URL of the device")
	user := flag.String("user", "admin", "Basic auth username")
	pass := flag.String("pass", "admin", "Basic auth password")
	authMode := flag.String("auth", "query", "How to send credentials: 'query' (?Basic=...) or 'header' (Authorization)")
	logPath := flag.String("log", "wssniff.jsonl", "Append every frame as a JSON line to this file ('' to disable)")
	flag.Parse()

	var logFile *os.File
	if *logPath != "" {
		var err error
		logFile, err = os.OpenFile(*logPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
		if err != nil {
			fmt.Fprintf(os.Stderr, "[wssniff] cannot open log %s: %v\n", *logPath, err)
			os.Exit(1)
		}
		defer logFile.Close()
		fmt.Fprintf(os.Stderr, "[wssniff] logging frames to %s\n", *logPath)
	}
	logEnc := func(e logEntry) {
		if logFile == nil {
			return
		}
		e.TS = time.Now().UTC().Format(time.RFC3339Nano)
		b, _ := json.Marshal(e)
		_, _ = logFile.Write(append(b, '\n'))
	}

	creds := base64.StdEncoding.EncodeToString([]byte(*user + ":" + *pass))

	target := *wsURL
	hdr := http.Header{}
	switch *authMode {
	case "query":
		u, err := url.Parse(target)
		if err != nil {
			fmt.Fprintln(os.Stderr, "bad url:", err)
			os.Exit(1)
		}
		q := u.Query()
		q.Set("Basic", creds)
		u.RawQuery = q.Encode()
		target = u.String()
	case "header":
		hdr.Set("Authorization", "Basic "+creds)
	default:
		fmt.Fprintln(os.Stderr, "auth must be 'query' or 'header'")
		os.Exit(1)
	}
	hdr.Set("Origin", strings.Replace(strings.Replace(target, "ws://", "http://", 1), "wss://", "https://", 1))

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// trap Ctrl-C so we can close the WS cleanly
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)
	go func() { <-sig; cancel() }()

	dialCtx, dialCancel := context.WithTimeout(ctx, 10*time.Second)
	defer dialCancel()

	fmt.Fprintf(os.Stderr, "[wssniff] dialing %s ...\n", target)
	logEnc(logEntry{Direction: "info", Note: "dialing " + target})
	c, _, err := websocket.Dial(dialCtx, target, &websocket.DialOptions{HTTPHeader: hdr})
	if err != nil {
		logEnc(logEntry{Direction: "error", Note: "dial failed: " + err.Error()})
		fmt.Fprintf(os.Stderr, "[wssniff] dial failed: %v\n", err)
		os.Exit(1)
	}
	defer c.Close(websocket.StatusNormalClosure, "bye")
	c.SetReadLimit(32 << 20) // 32MB; image frames could be large

	fmt.Fprintln(os.Stderr, "[wssniff] connected. Type JSON commands on stdin to send. Ctrl-C to quit.")
	logEnc(logEntry{Direction: "info", Note: "connected"})

	// reader: print every incoming frame
	go func() {
		for {
			typ, data, err := c.Read(ctx)
			if err != nil {
				logEnc(logEntry{Direction: "error", Note: "read closed: " + err.Error()})
				fmt.Fprintf(os.Stderr, "\n[wssniff] read closed: %v\n", err)
				cancel()
				return
			}
			ts := time.Now().Format("15:04:05.000")
			if typ == websocket.MessageBinary {
				fmt.Printf("[%s] <- BINARY %d bytes: %x...\n", ts, len(data), trim(data, 64))
				logEnc(logEntry{Direction: "recv", Type: "binary", Bytes: len(data), Hex: fmt.Sprintf("%x", trim(data, 64))})
				continue
			}
			fmt.Printf("[%s] <- %s\n", ts, string(data))
			e := logEntry{Direction: "recv", Type: "text", Bytes: len(data), Text: string(data)}
			if json.Valid(data) {
				e.JSON = json.RawMessage(data)
				e.Text = "" // avoid duplicating
			}
			logEnc(e)
		}
	}()

	// pinger: keep the connection alive
	go func() {
		t := time.NewTicker(20 * time.Second)
		defer t.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-t.C:
				pingCtx, pcancel := context.WithTimeout(ctx, 5*time.Second)
				if err := c.Ping(pingCtx); err != nil {
					fmt.Fprintf(os.Stderr, "[wssniff] ping failed: %v\n", err)
				}
				pcancel()
			}
		}
	}()

	// stdin: forward typed lines as text frames
	in := bufio.NewScanner(os.Stdin)
	in.Buffer(make([]byte, 0, 1<<20), 16<<20)
	for in.Scan() {
		line := strings.TrimSpace(in.Text())
		if line == "" {
			continue
		}
		writeCtx, wcancel := context.WithTimeout(ctx, 5*time.Second)
		if err := c.Write(writeCtx, websocket.MessageText, []byte(line)); err != nil {
			logEnc(logEntry{Direction: "error", Note: "write failed: " + err.Error()})
			fmt.Fprintf(os.Stderr, "[wssniff] write failed: %v\n", err)
			wcancel()
			cancel()
			break
		}
		wcancel()
		fmt.Printf("[%s] -> %s\n", time.Now().Format("15:04:05.000"), line)
		e := logEntry{Direction: "send", Type: "text", Bytes: len(line), Text: line}
		if json.Valid([]byte(line)) {
			e.JSON = json.RawMessage(line)
			e.Text = ""
		}
		logEnc(e)
	}
	<-ctx.Done()
}

func trim(b []byte, n int) []byte {
	if len(b) > n {
		return b[:n]
	}
	return b
}
