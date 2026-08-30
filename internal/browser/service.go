package browser

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

const (
	maxArtifactBytes = 8 * 1024 * 1024
	maxUploadBytes   = 128 * 1024 * 1024
)

// Driver is the controller-side browser engine boundary. The production
// implementation is a pinned local Playwright sidecar; tests inject this
// interface so policy behavior is independently verifiable without Chromium.
type Driver interface {
	SetOrigins(context.Context, []Origin) error
	CandidateTabs(context.Context) ([]Tab, error)
	Snapshot(context.Context, string) (Snapshot, error)
	Screenshot(context.Context, string) ([]byte, error)
	Navigate(context.Context, string, string) (Snapshot, error)
	Click(context.Context, string, string) (Snapshot, error)
	Fill(context.Context, string, string, string) (Snapshot, error)
	Select(context.Context, string, string, string) (Snapshot, error)
	Press(context.Context, string, string) (Snapshot, error)
	Download(context.Context, string, string) (Download, error)
	Upload(context.Context, string, string, string) (Snapshot, error)
	Close() error
}

// Download is browser data before the service stores it under the private
// artifact directory. The driver never receives an agent-selected path.
type Download struct {
	Name string
	Data []byte
}

// ServiceConfig starts one private local controller for a running session.
// Token must contain unpredictable bytes and is checked on every request.
type ServiceConfig struct {
	Store     *Store
	SessionID string
	Token     []byte
	Driver    Driver
	Socket    string
}

// Service owns a live browser driver and its private local socket. It is not a
// network service: Unix-socket permissions and a per-session bearer token are
// both required.
type Service struct {
	store     *Store
	sessionID string
	token     []byte
	driver    Driver
	socket    string
	listener  net.Listener
	mu        sync.Mutex
	uploads   map[string]string
}

func NewService(config ServiceConfig) (*Service, error) {
	if config.Store == nil || config.Driver == nil {
		return nil, errors.New("browser service requires a store and driver")
	}
	if err := sessionError(config.SessionID); err != nil {
		return nil, err
	}
	if len(config.Token) < 32 {
		return nil, errors.New("browser service token must contain at least 32 bytes")
	}
	if !filepath.IsAbs(config.Socket) {
		return nil, errors.New("browser service socket must be absolute")
	}
	session, err := config.Store.Get(config.SessionID)
	if err != nil {
		return nil, err
	}
	if session.State != StateRunning {
		return nil, ErrStopped
	}
	return &Service{store: config.Store, sessionID: config.SessionID, token: append([]byte(nil), config.Token...), driver: config.Driver, socket: config.Socket, uploads: make(map[string]string)}, nil
}

// Run serves until context cancellation or a Stop request. It serializes all
// requests through one controller lock so an agent cannot race a user takeover
// or another granted run on the same selected tab.
func (service *Service) Run(ctx context.Context) error {
	if err := os.MkdirAll(filepath.Dir(service.socket), 0o700); err != nil {
		return fmt.Errorf("create browser socket directory: %w", err)
	}
	if err := os.Chmod(filepath.Dir(service.socket), 0o700); err != nil {
		return fmt.Errorf("protect browser socket directory: %w", err)
	}
	_ = os.Remove(service.socket)
	listener, err := net.Listen("unix", service.socket)
	if err != nil {
		return fmt.Errorf("listen on browser socket: %w", err)
	}
	service.listener = listener
	if err := os.Chmod(service.socket, 0o600); err != nil {
		_ = listener.Close()
		return fmt.Errorf("protect browser socket: %w", err)
	}
	defer func() {
		_ = listener.Close()
		_ = os.Remove(service.socket)
		_ = service.driver.Close()
		_, _ = service.store.Stop(service.sessionID)
		removeToken(service.store, service.sessionID)
	}()
	if session, err := service.store.Get(service.sessionID); err != nil {
		return err
	} else if err := service.driver.SetOrigins(ctx, session.Origins); err != nil {
		return fmt.Errorf("apply browser origin policy: %w", err)
	}
	go func() {
		<-ctx.Done()
		_ = listener.Close()
	}()
	for {
		connection, err := listener.Accept()
		if err != nil {
			if ctx.Err() != nil || errors.Is(err, net.ErrClosed) {
				return nil
			}
			return fmt.Errorf("accept browser socket request: %w", err)
		}
		go service.serveConnection(ctx, connection)
	}
}

func (service *Service) serveConnection(ctx context.Context, connection net.Conn) {
	defer connection.Close()
	_ = connection.SetDeadline(time.Now().Add(30 * time.Second))
	request, err := decodeRequest(connection)
	if err != nil {
		_ = encodeResponse(connection, rpcResponse{Error: err.Error()})
		return
	}
	if !tokenEqual(request.Token, service.token) {
		_ = encodeResponse(connection, rpcResponse{ID: request.ID, Error: "browser session authentication failed"})
		return
	}
	service.mu.Lock()
	response := service.handle(ctx, request)
	service.mu.Unlock()
	_ = encodeResponse(connection, response)
}

func (service *Service) handle(ctx context.Context, request rpcRequest) rpcResponse {
	response := rpcResponse{ID: request.ID}
	var result any
	var err error
	switch request.Method {
	case "session":
		result, err = service.store.Get(service.sessionID)
	case "tabs":
		var session Session
		session, err = service.store.Get(service.sessionID)
		if err == nil {
			result = session.SelectedTabs
		}
	case "candidate_tabs":
		result, err = service.driver.CandidateTabs(ctx)
	case "select_tabs":
		var parameters struct {
			Tabs []Tab `json:"tabs"`
		}
		err = decodeParams(request.Params, &parameters)
		if err == nil {
			result, err = service.store.SelectTabs(service.sessionID, parameters.Tabs)
		}
	case "add_origin":
		var parameters struct {
			URL string `json:"url"`
		}
		err = decodeParams(request.Params, &parameters)
		if err == nil {
			var session Session
			session, err = service.store.AddOrigin(ctx, service.sessionID, parameters.URL, nil)
			if err == nil {
				err = service.driver.SetOrigins(ctx, session.Origins)
			}
			result = session
		}
	case "remove_origin":
		var parameters struct {
			URL string `json:"url"`
		}
		err = decodeParams(request.Params, &parameters)
		if err == nil {
			var session Session
			session, err = service.store.RemoveOrigin(service.sessionID, parameters.URL)
			if err == nil {
				err = service.driver.SetOrigins(ctx, session.Origins)
			}
			result = session
		}
	case "set_visual_capture":
		var parameters struct {
			Allowed bool `json:"allowed"`
		}
		err = decodeParams(request.Params, &parameters)
		if err == nil {
			result, err = service.store.SetVisualCapture(service.sessionID, parameters.Allowed)
		}
	case "allow_upload":
		var parameters struct {
			Path string `json:"path"`
		}
		err = decodeParams(request.Params, &parameters)
		if err == nil {
			result, err = service.allowUpload(parameters.Path)
		}
	case "snapshot":
		var parameters struct {
			TabID string `json:"tab_id"`
		}
		err = decodeParams(request.Params, &parameters)
		if err == nil {
			err = service.requireSelected(parameters.TabID)
		}
		if err == nil {
			result, err = service.driver.Snapshot(ctx, parameters.TabID)
		}
	case "screenshot":
		var parameters struct {
			TabID string `json:"tab_id"`
		}
		err = decodeParams(request.Params, &parameters)
		if err == nil {
			result, err = service.screenshot(ctx, parameters.TabID)
		}
	case "navigate":
		var parameters struct {
			TabID string `json:"tab_id"`
			URL   string `json:"url"`
		}
		err = decodeParams(request.Params, &parameters)
		if err == nil {
			err = service.requireURL(parameters.TabID, parameters.URL)
		}
		if err == nil {
			result, err = service.driver.Navigate(ctx, parameters.TabID, parameters.URL)
		}
	case "click", "fill", "select", "press", "download", "upload":
		result, err = service.action(ctx, request.Method, request.Params)
	case "stop":
		result, err = service.store.Stop(service.sessionID)
		if err == nil {
			go func() {
				if service.listener != nil {
					_ = service.listener.Close()
				}
			}()
		}
	default:
		err = errors.New("browser controller method is unsupported")
	}
	if err != nil {
		response.Error = err.Error()
		return response
	}
	response.Result = result
	return response
}

func (service *Service) action(ctx context.Context, method string, raw []byte) (any, error) {
	switch method {
	case "click":
		var parameters struct {
			TabID string `json:"tab_id"`
			Ref   string `json:"ref"`
		}
		if err := decodeParams(raw, &parameters); err != nil {
			return nil, err
		}
		if err := service.requireSelected(parameters.TabID); err != nil {
			return nil, err
		}
		return service.driver.Click(ctx, parameters.TabID, parameters.Ref)
	case "fill":
		var parameters struct {
			TabID string `json:"tab_id"`
			Ref   string `json:"ref"`
			Value string `json:"value"`
		}
		if err := decodeParams(raw, &parameters); err != nil {
			return nil, err
		}
		if err := service.requireSelected(parameters.TabID); err != nil {
			return nil, err
		}
		return service.driver.Fill(ctx, parameters.TabID, parameters.Ref, parameters.Value)
	case "select":
		var parameters struct {
			TabID string `json:"tab_id"`
			Ref   string `json:"ref"`
			Value string `json:"value"`
		}
		if err := decodeParams(raw, &parameters); err != nil {
			return nil, err
		}
		if err := service.requireSelected(parameters.TabID); err != nil {
			return nil, err
		}
		return service.driver.Select(ctx, parameters.TabID, parameters.Ref, parameters.Value)
	case "press":
		var parameters struct {
			TabID string `json:"tab_id"`
			Key   string `json:"key"`
		}
		if err := decodeParams(raw, &parameters); err != nil {
			return nil, err
		}
		if err := service.requireSelected(parameters.TabID); err != nil {
			return nil, err
		}
		return service.driver.Press(ctx, parameters.TabID, parameters.Key)
	case "download":
		var parameters struct {
			TabID string `json:"tab_id"`
			Ref   string `json:"ref"`
		}
		if err := decodeParams(raw, &parameters); err != nil {
			return nil, err
		}
		if err := service.requireSelected(parameters.TabID); err != nil {
			return nil, err
		}
		download, err := service.driver.Download(ctx, parameters.TabID, parameters.Ref)
		if err != nil {
			return nil, err
		}
		return service.writeArtifact("download", download.Name, download.Data)
	case "upload":
		var parameters struct {
			TabID    string `json:"tab_id"`
			Ref      string `json:"ref"`
			UploadID string `json:"upload_id"`
		}
		if err := decodeParams(raw, &parameters); err != nil {
			return nil, err
		}
		if err := service.requireSelected(parameters.TabID); err != nil {
			return nil, err
		}
		path, found := service.uploads[parameters.UploadID]
		if !found {
			return nil, errors.New("browser upload id is not registered for this local session")
		}
		return service.driver.Upload(ctx, parameters.TabID, parameters.Ref, path)
	default:
		return nil, errors.New("browser action is unsupported")
	}
}

func (service *Service) screenshot(ctx context.Context, tabID string) (Capture, error) {
	if err := service.requireSelected(tabID); err != nil {
		return Capture{}, err
	}
	session, err := service.store.Get(service.sessionID)
	if err != nil {
		return Capture{}, err
	}
	if !session.VisualCapture {
		return Capture{}, errors.New("developer has not enabled model-visible screenshots for this browser session")
	}
	contents, err := service.driver.Screenshot(ctx, tabID)
	if err != nil {
		return Capture{}, err
	}
	artifact, err := service.writeArtifact("screenshot", "screenshot.png", contents)
	if err != nil {
		return Capture{}, err
	}
	return Capture{Artifact: artifact, PNG: contents}, nil
}

func (service *Service) allowUpload(value string) (struct {
	Session Session `json:"session"`
	Upload  Upload  `json:"upload"`
}, error) {
	path, err := canonicalRegularFile(value, maxUploadBytes)
	if err != nil {
		return struct {
			Session Session `json:"session"`
			Upload  Upload  `json:"upload"`
		}{}, err
	}
	session, upload, err := service.store.AllowUpload(service.sessionID, filepath.Base(path))
	if err != nil {
		return struct {
			Session Session `json:"session"`
			Upload  Upload  `json:"upload"`
		}{}, err
	}
	service.uploads[upload.ID] = path
	return struct {
		Session Session `json:"session"`
		Upload  Upload  `json:"upload"`
	}{Session: session, Upload: upload}, nil
}

func (service *Service) requireSelected(tabID string) error {
	session, err := service.store.Get(service.sessionID)
	if err != nil {
		return err
	}
	for _, tab := range session.SelectedTabs {
		if tab.ID == tabID {
			return nil
		}
	}
	return errors.New("browser tab is not developer-selected for this session")
}

func (service *Service) requireURL(tabID, value string) error {
	if err := service.requireSelected(tabID); err != nil {
		return err
	}
	session, err := service.store.Get(service.sessionID)
	if err != nil {
		return err
	}
	if !AllowsURL(session.Origins, value) {
		return errors.New("browser URL origin is not approved for this session")
	}
	return nil
}

func (service *Service) writeArtifact(kind, name string, contents []byte) (Artifact, error) {
	if len(contents) == 0 || len(contents) > maxArtifactBytes {
		return Artifact{}, fmt.Errorf("browser %s artifact must contain 1-%d bytes", kind, maxArtifactBytes)
	}
	name = safeArtifactName(name)
	if name == "" {
		return Artifact{}, errors.New("browser artifact name is invalid")
	}
	digest := sha256.Sum256(append([]byte(kind+"\x00"), contents...))
	id := hex.EncodeToString(digest[:16])
	directory := filepath.Join(service.store.Directory(), "artifacts", service.sessionID)
	if err := os.MkdirAll(directory, 0o700); err != nil {
		return Artifact{}, fmt.Errorf("create browser artifact directory: %w", err)
	}
	path := filepath.Join(directory, id+"-"+name)
	if err := os.WriteFile(path, contents, 0o600); err != nil {
		return Artifact{}, fmt.Errorf("write browser artifact: %w", err)
	}
	return Artifact{ID: id, SessionID: service.sessionID, Kind: kind, Name: name, Bytes: int64(len(contents)), CreatedAt: service.store.now().UTC()}, nil
}

func canonicalRegularFile(value string, maxBytes int64) (string, error) {
	if strings.TrimSpace(value) == "" || !filepath.IsAbs(value) {
		return "", errors.New("browser upload path must be an absolute developer-selected file")
	}
	resolved, err := filepath.EvalSymlinks(value)
	if err != nil {
		return "", fmt.Errorf("resolve browser upload file: %w", err)
	}
	info, err := os.Stat(resolved)
	if err != nil {
		return "", fmt.Errorf("stat browser upload file: %w", err)
	}
	if !info.Mode().IsRegular() || info.Size() > maxBytes {
		return "", fmt.Errorf("browser upload must be a regular file no larger than %d MiB", maxBytes/(1024*1024))
	}
	return resolved, nil
}

func safeArtifactName(value string) string {
	value = filepath.Base(strings.TrimSpace(value))
	if value == "." || value == "" || len(value) > 160 || strings.ContainsAny(value, "\x00\r\n") {
		return ""
	}
	return value
}

func socketPath(store *Store, sessionID string) string {
	digest := sha256.Sum256([]byte(sessionID))
	// Unix-domain socket paths are capped at roughly 104 bytes on macOS and
	// 108 bytes on Linux. Gator state directories can legitimately be much
	// longer (for example a test or managed-home path), so use a short,
	// per-UID 0700 directory. The capability token remains under private Gator
	// state and authenticates every request independently of this path.
	return filepath.Join(os.TempDir(), fmt.Sprintf("gator-browser-%d", os.Getuid()), hex.EncodeToString(digest[:12])+".sock")
}

// SocketPath returns the private per-session Unix socket path. The socket is
// authenticated separately by a token stored in Gator's private state.
func SocketPath(store *Store, sessionID string) string { return socketPath(store, sessionID) }

func tokenPath(store *Store, sessionID string) string {
	digest := sha256.Sum256([]byte(sessionID))
	return filepath.Join(store.Directory(), "runtime", hex.EncodeToString(digest[:12])+".token")
}

// TokenPath is intended only for the command parent and its daemon child. The
// token contents are never displayed, persisted in a session record, or sent
// to a model provider.
func TokenPath(store *Store, sessionID string) string { return tokenPath(store, sessionID) }

// CreateToken writes a fresh private session capability for a daemon parent.
// The token file is removed when the daemon stops; it never enters session
// metadata or a run journal.
func CreateToken(store *Store, sessionID string) (string, []byte, error) {
	if store == nil {
		return "", nil, errors.New("browser store is required")
	}
	if err := sessionError(sessionID); err != nil {
		return "", nil, err
	}
	directory := filepath.Dir(tokenPath(store, sessionID))
	if err := os.MkdirAll(directory, 0o700); err != nil {
		return "", nil, fmt.Errorf("create browser runtime directory: %w", err)
	}
	if err := os.Chmod(directory, 0o700); err != nil {
		return "", nil, fmt.Errorf("protect browser runtime directory: %w", err)
	}
	contents := make([]byte, 32)
	if _, err := io.ReadFull(rand.Reader, contents); err != nil {
		return "", nil, fmt.Errorf("generate browser session token: %w", err)
	}
	path := tokenPath(store, sessionID)
	if err := os.WriteFile(path, contents, 0o600); err != nil {
		return "", nil, fmt.Errorf("write browser session token: %w", err)
	}
	return path, contents, nil
}

func readToken(store *Store, sessionID string) ([]byte, error) {
	path := tokenPath(store, sessionID)
	info, err := os.Lstat(path)
	if err != nil {
		return nil, fmt.Errorf("read browser session token: %w", err)
	}
	if !info.Mode().IsRegular() || info.Mode().Perm() != 0o600 {
		return nil, errors.New("browser session token must be a private regular file")
	}
	contents, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read browser session token: %w", err)
	}
	if len(contents) != 32 {
		return nil, errors.New("browser session token is invalid")
	}
	return contents, nil
}

// ReadToken reads the exact private token created for a local daemon.
func ReadToken(store *Store, sessionID string) ([]byte, error) { return readToken(store, sessionID) }

func removeToken(store *Store, sessionID string) { _ = os.Remove(tokenPath(store, sessionID)) }

func sortArtifacts(artifacts []Artifact) {
	sort.Slice(artifacts, func(left, right int) bool { return artifacts[left].CreatedAt.After(artifacts[right].CreatedAt) })
}
