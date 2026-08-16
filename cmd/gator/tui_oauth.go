package main

import (
	"context"
	"fmt"

	"github.com/gongahkia/gator/internal/auth"
	"github.com/gongahkia/gator/internal/model"
	"github.com/gongahkia/gator/internal/tui"
)

type tuiOAuthLogin struct {
	provider    model.Provider
	attempt     auth.BrowserAttempt
	callback    *auth.Callback
	credentials auth.Store
}

func beginTUIOAuthLogin(providerName string) (tui.OAuthLogin, error) {
	provider, err := model.ParseProvider(providerName)
	if err != nil {
		return nil, err
	}
	if !model.SupportsOAuthLogin(provider) {
		return nil, fmt.Errorf("provider %q does not use subscription OAuth", provider)
	}
	if provider == model.Copilot {
		return beginCopilotDeviceLogin()
	}
	if provider == model.XAI {
		return beginXAIDeviceLogin()
	}
	if provider == model.OpenRouter {
		return beginOpenRouterLogin()
	}
	if provider == model.KimiCoding {
		return beginKimiCodingDeviceLogin()
	}
	flow, err := oauthFlow(provider)
	if err != nil {
		return nil, err
	}
	attempt, err := auth.BeginBrowserFlow(flow)
	if err != nil {
		return nil, err
	}
	callback, err := attempt.StartCallback()
	if err != nil {
		return nil, err
	}
	credentials, err := gatorCredentials()
	if err != nil {
		callback.Close()
		return nil, err
	}
	return &tuiOAuthLogin{provider: provider, attempt: attempt, callback: callback, credentials: credentials}, nil
}

func (l *tuiOAuthLogin) URL() string {
	return l.attempt.AuthorizationURL()
}

func (l *tuiOAuthLogin) Complete(ctx context.Context) error {
	defer l.callback.Close()
	code, err := l.callback.Wait(ctx)
	if err != nil {
		return err
	}
	credential, err := l.attempt.Exchange(ctx, code)
	if err != nil {
		return err
	}
	if err := l.credentials.Put(string(l.provider), credential); err != nil {
		return fmt.Errorf("store OAuth credential: %w", err)
	}
	return nil
}

func (l *tuiOAuthLogin) Cancel() {
	if l != nil && l.callback != nil {
		l.callback.Close()
	}
}
