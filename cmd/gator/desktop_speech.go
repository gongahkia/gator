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
	Action      string            `json:"action"`
	Recording   *speech.Recording `json:"recording,omitempty"`
	Preferences json.RawMessage   `json:"preferences,omitempty"`
}
type desktopVoiceStatus struct {
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
		}
	}
	status := desktopVoiceStatus{Provider: provider, Model: selected, Preferences: voice, MaxSeconds: 120, MaxAudioBytes: speech.MaxAudioBytes}
	preferences, _ := json.Marshal(voice)
	status.Route = fmt.Sprintf("%x", sha256.Sum256([]byte(provider+"\x00"+selected+"\x00"+settings.ProviderEndpoint(provider)+"\x00"+string(preferences))))
	if provider != "gemini" && provider != "openai" {
		status.Issue = "Choose Gemini or OpenAI for dictation. API keys are managed in Settings → Models."
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
	if !status.Ready {
		status.Issue = "Configure the " + provider + " API key in Settings → Models to use dictation."
	}
	return speech.Config{Provider: provider, Model: selected, APIKey: key, BaseURL: settings.ProviderEndpoint(provider)}, status, nil
}

func runDesktopSpeech(in io.Reader, out io.Writer) error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	ctx, cancel := context.WithTimeout(ctx, 65*time.Second)
	defer cancel()
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
