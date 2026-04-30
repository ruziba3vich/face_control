package hasdk

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"strconv"
	"time"
)

// HTTPClient calls the FaceGate device's HTTP JSON API directly. The device
// listens on port 8000 (configurable via Extra.httpservice_port) and accepts
// POST / with JSON {"cmd":"...","version":"0.2",...}. Auth is HTTP Basic.
//
// Documented commands we use:
//   - "add person jpg"     register a face from a JPEG image
//   - "update person jpg"  modify an existing face (image + metadata)
//   - "delete person(s)"   delete by id (flag=-1) or by role
//   - "request persons"    list/query
//   - "get params"         a cheap sanity-check used by Ping
//
// The device's `id` field is capped at 19 bytes — too short for a UUID — so
// we keep a short surrogate (hasdk.NewFaceID) in `id` and stash the original
// UUID in `customer_text` (capped at 68 bytes). This matches the vendor's own
// recommendation in the API doc.
type HTTPClient struct {
	hc  *http.Client
	log *slog.Logger
}

func NewHTTPClient(log *slog.Logger) *HTTPClient {
	if log == nil {
		log = slog.Default()
	}
	return &HTTPClient{
		hc: &http.Client{
			Timeout: 30 * time.Second,
			Transport: &http.Transport{
				DialContext:           (&net.Dialer{Timeout: 5 * time.Second, KeepAlive: 30 * time.Second}).DialContext,
				MaxIdleConns:          16,
				IdleConnTimeout:       90 * time.Second,
				TLSHandshakeTimeout:   5 * time.Second,
				ExpectContinueTimeout: 1 * time.Second,
				DisableCompression:    true, // device docs forbid chunked transfer-encoding; keep request bodies plain
			},
		},
		log: log,
	}
}

// envelope is the response shape the device always returns.
type envelope struct {
	Reply    string          `json:"reply"`
	Cmd      string          `json:"cmd"`
	Code     int             `json:"code"`
	DeviceSN string          `json:"device_sn,omitempty"`
	ID       string          `json:"id,omitempty"`
	Desc     string          `json:"desc,omitempty"`
	Params   json.RawMessage `json:"params,omitempty"`
}

// APIError carries a non-zero code from the device.
type APIError struct {
	Cmd  string
	Code int
	Desc string
}

func (e *APIError) Error() string {
	if e.Desc != "" {
		return fmt.Sprintf("device %q failed (code=%d): %s", e.Cmd, e.Code, e.Desc)
	}
	return fmt.Sprintf("device %q failed (code=%d): %s", e.Cmd, e.Code, codeMessage(e.Code))
}

// codeMessage maps the documented error codes to human-readable messages.
// Source: HTTP_En.pdf section 1.6 + per-command response sections.
func codeMessage(code int) string {
	switch code {
	case 0:
		return "ok"
	case 1:
		return "protocol version mismatch"
	case 2:
		return "unsupported command"
	case 3:
		return "request contains illegal fields"
	case 4:
		return "authentication failed"
	case 5:
		return "system busy"
	case 6:
		return "insufficient resources"
	case 20:
		return "data entry reached upper limit"
	case 21:
		return "record already exists"
	case 22:
		return "record does not exist"
	case 25:
		return "failed to extract face feature (no face in image)"
	case 35:
		return "image decoding failed"
	case 36:
		return "image too large (>10MB)"
	case 37:
		return "normalization failed"
	case 38:
		return "face too small"
	case 39:
		return "portrait quality too poor"
	case 40:
		return "more than one face in image"
	case 41:
		return "face is incomplete in image"
	}
	return "unknown error"
}

const protocolVersion = "0.2"

func (c *HTTPClient) post(ctx context.Context, dev Device, body map[string]any, want string) (*envelope, error) {
	body["version"] = protocolVersion
	if _, ok := body["cmd"]; !ok {
		return nil, fmt.Errorf("post: cmd missing")
	}

	raw, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("marshal: %w", err)
	}
	url := "http://" + net.JoinHostPort(dev.Host, strconv.Itoa(int(dev.Port))) + "/"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(raw))
	if err != nil {
		return nil, fmt.Errorf("new request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json; charset=utf-8")
	req.ContentLength = int64(len(raw)) // explicit; device docs forbid chunked encoding
	if dev.Username != "" || dev.Password != "" {
		req.SetBasicAuth(dev.Username, dev.Password)
	}

	resp, err := c.hc.Do(req)
	if err != nil {
		return nil, fmt.Errorf("post %s: %w", body["cmd"], err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(io.LimitReader(resp.Body, 32<<20))
	if err != nil {
		return nil, fmt.Errorf("read body: %w", err)
	}
	if resp.StatusCode/100 != 2 {
		return nil, fmt.Errorf("device returned http %d: %s", resp.StatusCode, truncateBytes(respBody, 200))
	}

	var env envelope
	if err := json.Unmarshal(respBody, &env); err != nil {
		return nil, fmt.Errorf("decode envelope: %w (body=%q)", err, truncateBytes(respBody, 200))
	}
	if env.Code != 0 {
		return &env, &APIError{Cmd: env.Cmd, Code: env.Code, Desc: env.Desc}
	}
	if want != "" && env.Cmd != "" && env.Cmd != want {
		c.log.Warn("hasdk.http unexpected cmd in reply", "want", want, "got", env.Cmd)
	}
	return &env, nil
}

// Register implements Client. Uses cmd="add person jpg" with cover=true so
// repeat calls overwrite cleanly (idempotent enrollment).
func (c *HTTPClient) Register(ctx context.Context, req RegisterRequest) error {
	role := int(req.Role)
	if role == 0 {
		// Per HTTP_En.pdf, role for face matching is 1=whitelist or 2=blacklist;
		// 0 is the "ordinary personnel" semantic that matters for query/delete
		// flags but registration expects a non-zero role.
		role = int(RoleWhitelisted)
	}
	body := map[string]any{
		"cmd":           "add person jpg",
		"id":            req.FaceID,
		"name":          req.FaceName,
		"role":          role,
		"kind":          0,
		"cover":         true,
		"image_num":     1,
		"reg_images": []map[string]string{{
			"format":     "jpg",
			"image_data": base64.StdEncoding.EncodeToString(req.JPEG),
		}},
	}
	if req.Device.ID != "" {
		// Stash full UUID for later cross-reference in capture events.
		body["customer_text"] = truncate(req.Device.ID, 68)
	}
	_, err := c.post(ctx, req.Device, body, "add person jpg")
	return err
}

// Delete implements Client. flag=-1 selects "delete by id".
func (c *HTTPClient) Delete(ctx context.Context, dev Device, faceID string) error {
	body := map[string]any{
		"cmd":  "delete person(s)",
		"flag": -1,
		"id":   faceID,
	}
	_, err := c.post(ctx, dev, body, "delete person(s)")
	if err != nil {
		// Treat "record does not exist" as success — caller's intent (gone) is satisfied.
		var apiErr *APIError
		if errors.As(err, &apiErr) && apiErr.Code == 22 {
			return nil
		}
		return err
	}
	return nil
}

func (c *HTTPClient) Close() error { return nil }

// Ping is a cheap reachability check; not part of the Client interface yet,
// but useful from main/healthchecks. Returns the device serial number.
func (c *HTTPClient) Ping(ctx context.Context, dev Device) (string, error) {
	env, err := c.post(ctx, dev, map[string]any{"cmd": "get params"}, "get params")
	if err != nil {
		return "", err
	}
	return env.DeviceSN, nil
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n]
}

func truncateBytes(b []byte, n int) string {
	if len(b) <= n {
		return string(b)
	}
	return string(b[:n]) + "..."
}

