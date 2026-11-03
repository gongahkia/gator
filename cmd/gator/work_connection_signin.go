package main

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/gongahkia/gator/internal/auth"
	"github.com/gongahkia/gator/internal/connector"
	"github.com/gongahkia/gator/internal/state"
)

// A sign-in remains host-owned. The renderer receives only a browser URL and
// status; OAuth state/verifier, tokens and client secret never enter a chat.
type connectionSignIn struct {
	mu     sync.Mutex
	active *connectionSignInAttempt
}
type connectionSignInAttempt struct {
	id, url, issue string
	notice         string
	done           bool
	cancel         context.CancelFunc
}
type connectionSignInStatus struct {
	ID, URL, Issue string
	Notice         string
	Done           bool
}

func (s *connectionSignIn) Status() connectionSignInStatus {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.active == nil {
		return connectionSignInStatus{}
	}
	a := s.active
	return connectionSignInStatus{ID: a.id, URL: a.url, Issue: a.issue, Notice: a.notice, Done: a.done}
}
func (s *connectionSignIn) Pending() string {
	status := s.Status()
	if !status.Done {
		return status.ID
	}
	return ""
}
func (s *connectionSignIn) Cancel(id string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.active != nil && s.active.id == id {
		s.active.cancel()
		s.active = nil
	}
}

func (s *connectionSignIn) Begin(stateDir, id, digest, clientSecret string) (connectionSignInStatus, error) {
	return s.begin(stateDir, id, digest, clientSecret, "", false)
}

func (s *connectionSignIn) EnableGmailSending(stateDir, id, digest, credentialDigest, clientSecret string) (connectionSignInStatus, error) {
	return s.begin(stateDir, id, digest, clientSecret, credentialDigest, true)
}

func (s *connectionSignIn) begin(stateDir, id, digest, clientSecret, credentialDigest string, enableSending bool) (connectionSignInStatus, error) {
	release, err := state.Guard(stateDir, false)
	if err != nil {
		return connectionSignInStatus{}, err
	}
	transferred := false
	defer func() {
		if !transferred {
			release()
		}
	}()
	descriptor, credentials, err := configuredConnector(id)
	if err != nil {
		return connectionSignInStatus{}, err
	}
	if digest == "" || digest != connectorLoginReview(descriptor) {
		return connectionSignInStatus{}, errors.New("connection changed; review sign-in again")
	}
	if descriptor.Authentication != connector.AuthOAuth {
		return connectionSignInStatus{}, errors.New("this connection uses a token, not browser sign-in")
	}
	if len(clientSecret) > 8192 || strings.ContainsAny(clientSecret, "\x00\r\n") {
		return connectionSignInStatus{}, errors.New("invalid client secret")
	}
	prior, found, err := credentials.Read(descriptor.CredentialRef())
	if err != nil {
		return connectionSignInStatus{}, err
	}
	var expected *auth.Credential
	if found {
		expected = &prior
		if clientSecret == "" {
			clientSecret = prior.OAuthClientSecret
		}
	}
	requested := descriptor
	if enableSending {
		if credentialDigest == "" || credentialDigest != credentialReview(expected) {
			return connectionSignInStatus{}, auth.ErrCredentialChanged
		}
		requested, err = gmailSendingDescriptor(descriptor, true)
		if err != nil {
			return connectionSignInStatus{}, err
		}
	}
	if clientSecret == "" && usesBundledGoogle(descriptor) {
		clientSecret = googleDesktopClientSecret
	}
	flow := auth.BrowserFlow{ClientID: descriptor.OAuthClientID, ClientSecret: clientSecret, AuthorizationURL: descriptor.OAuthAuthorizeURL, TokenURL: descriptor.OAuthTokenURL, RedirectURL: descriptor.OAuthRedirectURL, Scopes: strings.Fields(requested.OAuthScopes), AllowMissingExpiry: true, RequireBearerToken: true}
	var attempt auth.BrowserAttempt
	var callback *auth.Callback
	if descriptor.Kind == connector.KindGoogle {
		flow.Scopes = withoutGooglePlannerScopes(flow.Scopes)
		flow.AuthorizeParams = map[string]string{"access_type": "offline", "prompt": "consent"}
		if enableSending {
			// Google may return an identity-scope alias. Require its existing API
			// permissions and the new send scope exactly; identity is checked by
			// the separate account/read verification rather than inferred here.
			for _, scope := range flow.Scopes {
				if scope == "openid" || strings.HasPrefix(scope, "https://www.googleapis.com/auth/") && scope != "https://www.googleapis.com/auth/userinfo.email" && scope != "https://www.googleapis.com/auth/userinfo.profile" {
					flow.RequiredScopes = append(flow.RequiredScopes, scope)
				}
			}
		}
		attempt, callback, err = auth.BeginEphemeralLoopbackFlow(flow, "/oauth/callback")
	} else {
		attempt, err = auth.BeginBrowserFlow(flow)
		if err == nil {
			callback, err = attempt.StartCallback()
		}
	}
	if err != nil {
		return connectionSignInStatus{}, fmt.Errorf("start sign-in: %w", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	next := &connectionSignInAttempt{id: id, url: attempt.AuthorizationURL(), cancel: cancel}
	s.mu.Lock()
	if s.active != nil && !s.active.done {
		s.mu.Unlock()
		cancel()
		callback.Close()
		return connectionSignInStatus{}, errors.New("finish or cancel the current sign-in first")
	}
	s.active = next
	s.mu.Unlock()
	transferred = true
	go func() {
		defer release()
		defer cancel()
		defer callback.Close()
		code, err := callback.Wait(ctx)
		var credential auth.Credential
		if err == nil {
			credential, err = attempt.Exchange(ctx, code)
		}
		if errors.Is(err, auth.ErrRequiredScopes) {
			err = errors.New("Google did not confirm the requested API permissions. The previous local connection is kept. Try again and review the access choices in your browser")
		} else if err != nil {
			err = errors.New("authorization could not be verified; check the OAuth app details and try again")
		}
		credential.OAuthClientSecret = clientSecret
		if err == nil && enableSending && credential.Refresh == "" {
			err = errors.New("Google did not provide a new offline grant; review access and try again")
		}
		s.mu.Lock()
		defer s.mu.Unlock()
		if s.active != next || ctx.Err() != nil {
			if s.active == next {
				next.done = true
				next.url = ""
				next.issue = "Sign-in timed out or was cancelled. Try again."
			}
			return
		}
		if err == nil {
			current, _, loadErr := configuredConnector(id)
			if loadErr != nil || connectorLoginReview(current) != digest {
				err = errors.New("connection changed during sign-in; review it and try again")
			}
		}
		if err == nil {
			if enableSending {
				err = publishGmailSendingPermission(descriptor, requested, credentials, expected, credential)
			} else {
				err = credentials.ReplaceReviewed(descriptor.CredentialRef(), expected, &credential)
			}
		}
		next.done = true
		next.url = ""
		if err != nil {
			next.issue = "Sign-in did not complete: " + err.Error()
		} else if enableSending {
			next.notice = "Email sending enabled for new tasks. Each send still needs your exact approval. Check account and read access to verify the selected account."
		}
	}()
	return s.Status(), nil
}
