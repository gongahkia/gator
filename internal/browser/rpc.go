package browser

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"sync/atomic"
	"time"
)

const maxRPCBytes = 9 * 1024 * 1024

type rpcRequest struct {
	ID     uint64          `json:"id"`
	Token  []byte          `json:"token"`
	Method string          `json:"method"`
	Params json.RawMessage `json:"params,omitempty"`
}

type rpcResponse struct {
	ID     uint64 `json:"id"`
	Result any    `json:"result,omitempty"`
	Error  string `json:"error,omitempty"`
}

func decodeRequest(reader io.Reader) (rpcRequest, error) {
	var request rpcRequest
	decoder := json.NewDecoder(io.LimitReader(reader, maxRPCBytes+1))
	if err := decoder.Decode(&request); err != nil {
		return rpcRequest{}, errors.New("browser controller request is invalid")
	}
	if request.ID == 0 || request.Method == "" || len(request.Token) == 0 {
		return rpcRequest{}, errors.New("browser controller request is incomplete")
	}
	return request, nil
}

func encodeResponse(writer io.Writer, response rpcResponse) error {
	return json.NewEncoder(writer).Encode(response)
}

func decodeParams(raw json.RawMessage, target any) error {
	if len(raw) == 0 {
		raw = []byte(`{}`)
	}
	decoder := json.NewDecoder(io.LimitReader(&rawReader{contents: raw}, maxRPCBytes+1))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return errors.New("browser controller arguments are invalid")
	}
	return nil
}

type rawReader struct{ contents []byte }

func (reader *rawReader) Read(destination []byte) (int, error) {
	if len(reader.contents) == 0 {
		return 0, io.EOF
	}
	count := copy(destination, reader.contents)
	reader.contents = reader.contents[count:]
	return count, nil
}

func tokenEqual(left, right []byte) bool {
	return len(left) == len(right) && subtle.ConstantTimeCompare(left, right) == 1
}

// Client is a short-lived authenticated Unix-socket client. It is safe to
// pass into an Executor: browser tool calls contain only opaque IDs and the
// client reads the local per-session capability itself.
type Client struct {
	store     *Store
	sessionID string
	socket    string
	sequence  atomic.Uint64
}

func NewClient(store *Store, sessionID string) (*Client, error) {
	if store == nil {
		return nil, errors.New("browser store is required")
	}
	if err := sessionError(sessionID); err != nil {
		return nil, err
	}
	return &Client{store: store, sessionID: sessionID, socket: socketPath(store, sessionID)}, nil
}

func (client *Client) Session(ctx context.Context, _ string) (Session, error) {
	var result Session
	err := client.call(ctx, "session", nil, &result)
	return result, err
}

func (client *Client) Tabs(ctx context.Context, _ string) ([]Tab, error) {
	var result []Tab
	err := client.call(ctx, "tabs", nil, &result)
	return result, err
}

func (client *Client) Snapshot(ctx context.Context, _ string, tabID string) (Snapshot, error) {
	var result Snapshot
	err := client.call(ctx, "snapshot", struct {
		TabID string `json:"tab_id"`
	}{tabID}, &result)
	return result, err
}

func (client *Client) Screenshot(ctx context.Context, _ string, tabID string) (Capture, error) {
	var result Capture
	err := client.call(ctx, "screenshot", struct {
		TabID string `json:"tab_id"`
	}{tabID}, &result)
	return result, err
}

func (client *Client) Navigate(ctx context.Context, _ string, tabID, target string) (Snapshot, error) {
	var result Snapshot
	err := client.call(ctx, "navigate", struct {
		TabID string `json:"tab_id"`
		URL   string `json:"url"`
	}{tabID, target}, &result)
	return result, err
}

func (client *Client) Click(ctx context.Context, _ string, tabID, ref string) (Snapshot, error) {
	var result Snapshot
	err := client.call(ctx, "click", struct {
		TabID string `json:"tab_id"`
		Ref   string `json:"ref"`
	}{tabID, ref}, &result)
	return result, err
}

func (client *Client) Fill(ctx context.Context, _ string, tabID, ref, value string) (Snapshot, error) {
	var result Snapshot
	err := client.call(ctx, "fill", struct {
		TabID string `json:"tab_id"`
		Ref   string `json:"ref"`
		Value string `json:"value"`
	}{tabID, ref, value}, &result)
	return result, err
}

func (client *Client) Select(ctx context.Context, _ string, tabID, ref, value string) (Snapshot, error) {
	var result Snapshot
	err := client.call(ctx, "select", struct {
		TabID string `json:"tab_id"`
		Ref   string `json:"ref"`
		Value string `json:"value"`
	}{tabID, ref, value}, &result)
	return result, err
}

func (client *Client) Press(ctx context.Context, _ string, tabID, key string) (Snapshot, error) {
	var result Snapshot
	err := client.call(ctx, "press", struct {
		TabID string `json:"tab_id"`
		Key   string `json:"key"`
	}{tabID, key}, &result)
	return result, err
}

func (client *Client) Download(ctx context.Context, _ string, tabID, ref string) (Artifact, error) {
	var result Artifact
	err := client.call(ctx, "download", struct {
		TabID string `json:"tab_id"`
		Ref   string `json:"ref"`
	}{tabID, ref}, &result)
	return result, err
}

func (client *Client) Upload(ctx context.Context, _ string, tabID, ref, uploadID string) (Snapshot, error) {
	var result Snapshot
	err := client.call(ctx, "upload", struct {
		TabID    string `json:"tab_id"`
		Ref      string `json:"ref"`
		UploadID string `json:"upload_id"`
	}{tabID, ref, uploadID}, &result)
	return result, err
}

func (client *Client) CandidateTabs(ctx context.Context) ([]Tab, error) {
	var result []Tab
	err := client.call(ctx, "candidate_tabs", nil, &result)
	return result, err
}

func (client *Client) SelectTabs(ctx context.Context, tabs []Tab) (Session, error) {
	var result Session
	err := client.call(ctx, "select_tabs", struct {
		Tabs []Tab `json:"tabs"`
	}{tabs}, &result)
	return result, err
}

func (client *Client) AddOrigin(ctx context.Context, value string) (Session, error) {
	var result Session
	err := client.call(ctx, "add_origin", struct {
		URL string `json:"url"`
	}{value}, &result)
	return result, err
}

func (client *Client) RemoveOrigin(ctx context.Context, value string) (Session, error) {
	var result Session
	err := client.call(ctx, "remove_origin", struct {
		URL string `json:"url"`
	}{value}, &result)
	return result, err
}

func (client *Client) SetVisualCapture(ctx context.Context, allowed bool) (Session, error) {
	var result Session
	err := client.call(ctx, "set_visual_capture", struct {
		Allowed bool `json:"allowed"`
	}{allowed}, &result)
	return result, err
}

func (client *Client) AllowUpload(ctx context.Context, path string) (Session, Upload, error) {
	var result struct {
		Session Session `json:"session"`
		Upload  Upload  `json:"upload"`
	}
	err := client.call(ctx, "allow_upload", struct {
		Path string `json:"path"`
	}{path}, &result)
	return result.Session, result.Upload, err
}

func (client *Client) Stop(ctx context.Context) (Session, error) {
	var result Session
	err := client.call(ctx, "stop", nil, &result)
	return result, err
}

// Artifacts lists private local capture metadata without ever reading or
// sending artifact contents. It is available only to the local OS user who
// can read Gator's browser state directory.
func (client *Client) Artifacts() ([]Artifact, error) {
	directory := filepath.Join(client.store.Directory(), "artifacts", client.sessionID)
	entries, err := os.ReadDir(directory)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("list browser artifacts: %w", err)
	}
	artifacts := make([]Artifact, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || !entry.Type().IsRegular() {
			continue
		}
		info, err := entry.Info()
		if err != nil || info.Mode().Perm()&0o077 != 0 {
			continue
		}
		name := entry.Name()
		parts := splitArtifactName(name)
		if parts.id == "" {
			continue
		}
		kind := "download"
		if stringsHasSuffix(parts.name, ".png") {
			kind = "screenshot"
		}
		artifacts = append(artifacts, Artifact{ID: parts.id, SessionID: client.sessionID, Kind: kind, Name: parts.name, Bytes: info.Size(), CreatedAt: info.ModTime().UTC()})
	}
	sortArtifacts(artifacts)
	return artifacts, nil
}

// ExportArtifact copies one private browser artifact to an explicitly chosen
// new absolute path. It refuses to overwrite an existing file and never lets a
// model supply the destination: only CLI/TUI developer controls call it.
func (client *Client) ExportArtifact(id, destination string) error {
	if len(id) != 32 {
		return errors.New("browser artifact id is invalid")
	}
	if !filepath.IsAbs(destination) || filepath.Clean(destination) != destination {
		return errors.New("browser artifact export path must be absolute and clean")
	}
	artifacts, err := client.Artifacts()
	if err != nil {
		return err
	}
	var artifact Artifact
	found := false
	for _, candidate := range artifacts {
		if candidate.ID == id {
			artifact = candidate
			found = true
			break
		}
	}
	if !found {
		return errors.New("browser artifact was not found")
	}
	sourcePath := filepath.Join(client.store.Directory(), "artifacts", client.sessionID, artifact.ID+"-"+artifact.Name)
	sourceInfo, err := os.Lstat(sourcePath)
	if err != nil || !sourceInfo.Mode().IsRegular() || sourceInfo.Mode().Perm()&0o077 != 0 {
		return errors.New("browser artifact is unavailable or no longer private")
	}
	source, err := os.Open(sourcePath)
	if err != nil {
		return fmt.Errorf("open browser artifact: %w", err)
	}
	defer source.Close()
	destinationFile, err := os.OpenFile(destination, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return fmt.Errorf("create browser artifact export: %w", err)
	}
	defer destinationFile.Close()
	if _, err := io.Copy(destinationFile, io.LimitReader(source, maxArtifactBytes+1)); err != nil {
		return fmt.Errorf("copy browser artifact: %w", err)
	}
	if err := destinationFile.Sync(); err != nil {
		return fmt.Errorf("sync browser artifact export: %w", err)
	}
	return nil
}

func (client *Client) call(ctx context.Context, method string, parameters any, target any) error {
	token, err := readToken(client.store, client.sessionID)
	if err != nil {
		return ErrUnavailable
	}
	payload, err := json.Marshal(parameters)
	if err != nil {
		return fmt.Errorf("encode browser controller request: %w", err)
	}
	request := rpcRequest{ID: client.sequence.Add(1), Token: token, Method: method, Params: payload}
	dialer := net.Dialer{Timeout: 5 * time.Second}
	connection, err := dialer.DialContext(ctx, "unix", client.socket)
	if err != nil {
		return ErrUnavailable
	}
	defer connection.Close()
	if deadline, found := ctx.Deadline(); found {
		_ = connection.SetDeadline(deadline)
	} else {
		_ = connection.SetDeadline(time.Now().Add(30 * time.Second))
	}
	if err := json.NewEncoder(connection).Encode(request); err != nil {
		return fmt.Errorf("write browser controller request: %w", err)
	}
	var response struct {
		ID     uint64          `json:"id"`
		Result json.RawMessage `json:"result"`
		Error  string          `json:"error"`
	}
	decoder := json.NewDecoder(io.LimitReader(connection, maxRPCBytes+1))
	if err := decoder.Decode(&response); err != nil {
		return fmt.Errorf("read browser controller response: %w", err)
	}
	if response.ID != request.ID || response.Error != "" {
		if response.Error != "" {
			return errors.New(response.Error)
		}
		return errors.New("browser controller returned an invalid response")
	}
	if target == nil {
		return nil
	}
	if err := json.Unmarshal(response.Result, target); err != nil {
		return fmt.Errorf("decode browser controller response: %w", err)
	}
	return nil
}

type artifactName struct {
	id   string
	name string
}

func splitArtifactName(value string) artifactName {
	if len(value) < 34 || value[32] != '-' {
		return artifactName{}
	}
	for _, character := range value[:32] {
		if !(character >= '0' && character <= '9' || character >= 'a' && character <= 'f') {
			return artifactName{}
		}
	}
	return artifactName{id: value[:32], name: value[33:]}
}

func stringsHasSuffix(value, suffix string) bool {
	return len(value) >= len(suffix) && value[len(value)-len(suffix):] == suffix
}
