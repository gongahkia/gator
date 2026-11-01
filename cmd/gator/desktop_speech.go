package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/signal"
	"runtime"
	"syscall"
	"time"

	"github.com/gongahkia/gator/internal/auth"
	"github.com/gongahkia/gator/internal/config"
	"github.com/gongahkia/gator/internal/model"
	"github.com/gongahkia/gator/internal/speech"
	"github.com/gongahkia/gator/internal/state"
	sharedusage "github.com/gongahkia/gator/internal/usage"
	"github.com/gongahkia/gator/internal/workrun"
)

type desktopSpeechRequest struct {
	Route       string            `json:"route,omitempty"`
	Model       string            `json:"model,omitempty"`
	Action      string            `json:"action"`
	Recording   *speech.Recording `json:"recording,omitempty"`
	Preferences json.RawMessage   `json:"preferences,omitempty"`
}
type desktopLocalVoice struct {
	Supported    bool                `json:"supported"`
	RuntimeReady bool                `json:"runtime_ready"`
	Models       []speech.LocalModel `json:"models"`
}
type desktopVoiceStatus struct {
	Local         *desktopLocalVoice   `json:"local,omitempty"`
	Route         string               `json:"route"`
	Ready         bool                 `json:"ready"`
	Issue         string               `json:"issue,omitempty"`
	Provider      string               `json:"provider"`
	Model         string               `json:"model"`
	Preferences   config.VoiceSettings `json:"preferences"`
	MaxSeconds    int                  `json:"max_seconds"`
	MaxAudioBytes int                  `json:"max_audio_bytes"`
}

func voiceConfiguration(settings config.Settings, stateDir string) (speech.Config, desktopVoiceStatus, error) {
	voice := settings.Desktop.Voice
	provider := voice.Provider
	if provider == "" {
		provider = settings.Defaults.Provider
	}
	selected := voice.Model
	if selected == "" {
		if provider == "gemini" {
			if settings.Defaults.Provider == "gemini" {
				selected = settings.Defaults.Model
			}
			selected = model.EffectiveModel(model.Gemini, selected)
		} else if provider == "openai" {
			selected = speech.DefaultOpenAIModel
		} else if provider == speech.LocalProvider {
			selected = speech.DefaultLocalModel
		}
	}
	status := desktopVoiceStatus{Provider: provider, Model: selected, Preferences: voice, MaxSeconds: 120, MaxAudioBytes: speech.MaxAudioBytes}
	preferences, _ := json.Marshal(voice)
	status.Route = fmt.Sprintf("%x", sha256.Sum256([]byte(provider+"\x00"+selected+"\x00"+settings.ProviderEndpoint(provider)+"\x00"+string(preferences))))
	if provider == speech.LocalProvider {
		local, err := speech.OpenLocalStore(stateDir)
		if err != nil {
			return speech.Config{}, status, err
		}
		executable, runtimeErr := speech.ResolveLocalRuntime()
		status.Local = &desktopLocalVoice{Supported: runtime.GOOS == "darwin" || runtime.GOOS == "linux", RuntimeReady: runtimeErr == nil, Models: local.Models()}
		present := false
		for _, model := range status.Local.Models {
			if model.ID == selected {
				present = model.Available
			}
		}
		if runtimeErr != nil {
			status.Issue = runtimeErr.Error()
		} else if !present {
			status.Issue = "Download the selected Whisper model below. No API key is needed."
		} else {
			status.Ready = true
		}
		status.Route = fmt.Sprintf("%x", sha256.Sum256([]byte(status.Route+"\x00"+local.Binding(selected, executable))))
		return speech.Config{Provider: provider, Model: selected, Local: &speech.LocalConfig{Store: local, Executable: executable}}, status, nil
	}
	if provider != "gemini" && provider != "openai" {
		status.Issue = "Choose local Whisper, Gemini or OpenAI for dictation. Local Whisper does not require an API key."
		return speech.Config{}, status, nil
	}
	credentials, err := auth.New(stateDir)
	if err != nil {
		return speech.Config{}, status, err
	}
	credential, found, err := credentials.Read(provider)
	if err != nil {
		return speech.Config{}, status, err
	}
	key := ""
	if found && credential.IsAPIKey() && !credential.Expired(time.Now()) {
		key = credential.Key
	}
	if key == "" {
		key = os.Getenv(model.APIKeyEnvironment(model.Provider(provider)))
	}
	status.Ready = key != ""
	if status.Ready {
		ledger, err := sharedusage.Open(stateDir)
		if err != nil {
			return speech.Config{}, status, err
		}
		if err := ledger.Admission(time.Now()); err != nil {
			status.Ready = false
			status.Issue = err.Error()
		}
	}
	if !status.Ready {
		if status.Issue == "" {
			status.Issue = "Configure the " + provider + " API key in Settings → Models to use dictation."
		}
	}
	return speech.Config{Provider: provider, Model: selected, APIKey: key, BaseURL: settings.ProviderEndpoint(provider)}, status, nil
}

func runDesktopSpeech(in io.Reader, out io.Writer) error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	data, err := io.ReadAll(io.LimitReader(in, 12*1024*1024+1))
	if err != nil {
		return err
	}
	if len(data) > 12*1024*1024 {
		return errors.New("voice request is too large")
	}
	var request desktopSpeechRequest
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&request); err != nil {
		return errors.New("invalid voice request")
	}
	if decoder.Decode(new(any)) != io.EOF {
		return errors.New("trailing voice request data")
	}
	timeout := 65 * time.Second
	if request.Action == "download" {
		timeout = 10 * time.Minute
	} else if request.Action == "transcribe" {
		timeout = speech.LocalTimeout + 5*time.Second
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	if request.Action != "download" && request.Model != "" {
		return errors.New("model selection is only valid for an explicit voice download")
	}
	root, err := state.ResolveDir("")
	if err != nil {
		return err
	}
	release, err := state.Guard(root, false)
	if err != nil {
		return err
	}
	defer release()
	store, err := config.DefaultStore()
	if err != nil {
		return err
	}
	if request.Action == "configure" {
		if request.Recording != nil {
			return errors.New("configuration cannot contain a recording")
		}
		controller := gatorAppController{stateDir: root, settings: store, request: &workrun.Request{}}
		mutation, err := controller.prepareDesktopPreferences(workrun.AppChange{Setting: "voice", Value: request.Preferences})
		if err != nil {
			return err
		}
		if _, err := mutation.Apply(ctx); err != nil {
			return err
		}
	} else if request.Preferences != nil {
		return errors.New("voice preferences require a configure request")
	}
	settings, err := store.Load()
	if err != nil {
		return err
	}
	backend, status, err := voiceConfiguration(settings, root)
	if err != nil {
		return err
	}
	if request.Action == "download" {
		if request.Recording != nil || status.Provider != speech.LocalProvider || status.Local == nil || !status.Local.Supported || request.Model != status.Model {
			return errors.New("select the local Whisper model before downloading it")
		}
		if err := backend.Local.Store.Download(ctx, request.Model); err != nil {
			return err
		}
		current, err := store.Load()
		if err != nil {
			return err
		}
		_, fresh, err := voiceConfiguration(current, root)
		if err != nil {
			return err
		}
		return json.NewEncoder(out).Encode(fresh)
	}
	if request.Action == "status" || request.Action == "configure" {
		if request.Recording != nil {
			return errors.New("voice status cannot contain audio")
		}
		return json.NewEncoder(out).Encode(status)
	}
	if request.Action != "transcribe" || request.Recording == nil {
		return errors.New("unsupported voice action")
	}
	if !status.Ready {
		return errors.New(status.Issue)
	}
	if request.Route != status.Route {
		return errors.New("voice settings changed during recording; review the new provider and record again")
	}
	request.Recording.Language = settings.Desktop.Voice.Language
	if err := speech.ValidateRecording(*request.Recording); err != nil {
		return err
	}
	if status.Provider == speech.LocalProvider {
		result, err := speech.Transcribe(ctx, backend, *request.Recording)
		if err != nil {
			return err
		}
		// Local inference has no provider token bill. Do not reserve fictional
		// API tokens based on audio size or silently retry through a cloud provider.
		return json.NewEncoder(out).Encode(result)
	}
	ledger, err := sharedusage.Open(root)
	if err != nil {
		return err
	}
	reserved := int64(len(request.Recording.Data)/32 + 4096)
	charge, err := ledger.Reserve(time.Now(), "voice", status.Provider+"/"+status.Model, reserved)
	if err != nil {
		return err
	}
	result, transcriptionErr := speech.Transcribe(ctx, backend, *request.Recording)
	tokens := result.Tokens
	if !result.Reported {
		tokens = reserved
	}
	if err := charge.Finish(tokens, result.Reported, transcriptionErr != nil, nil); err != nil {
		return err
	}
	if transcriptionErr != nil {
		return transcriptionErr
	}
	return json.NewEncoder(out).Encode(result)
}
