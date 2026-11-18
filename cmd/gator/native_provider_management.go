package main

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"net/url"
	"reflect"
	"strings"
	"sync"
	"time"

	"github.com/gongahkia/gator/internal/auth"
	"github.com/gongahkia/gator/internal/config"
	"github.com/gongahkia/gator/internal/model"
	"github.com/gongahkia/gator/internal/modelcatalog"
)

var errProviderReviewChanged = errors.New("provider review changed")

type nativeProviderReview struct {
	public     modelcatalog.NativeProviderReview
	settings   config.Settings
	credential *auth.Credential
	setup      modelcatalog.CloudModelSetup
	replace    bool
	expires    time.Time
}
type nativeProviderBackend struct {
	mu       sync.Mutex
	settings config.Store
	stateDir string
	review   *nativeProviderReview
}
type reviewedCloudSettings struct {
	store    config.Store
	expected config.Settings
}

func (s reviewedCloudSettings) Update(change func(*config.Settings) error) error {
	return s.store.Update(func(current *config.Settings) error {
		if !reflect.DeepEqual(*current, s.expected) {
			return errProviderReviewChanged
		}
		return change(current)
	})
}
func providerNonce() string {
	var b [24]byte
	if _, err := rand.Read(b[:]); err != nil {
		return ""
	}
	return hex.EncodeToString(b[:])
}
func supportedNativeProvider(name string) (model.Provider, bool) {
	p, err := model.ParseProvider(name)
	return p, err == nil && string(p) == name && model.SupportsAPIKeyLogin(p)
}

func safeProviderURL(value string, azure bool) bool {
	if value == "" {
		return true
	}
	u, err := url.Parse(value)
	if err != nil || u.User != nil || u.Hostname() == "" || (u.Scheme != "https" && u.Scheme != "http") || u.Fragment != "" {
		return false
	}
	for key := range u.Query() {
		if !azure || key != "api-version" {
			return false
		}
	}
	return true
}
func providerSecretValues(c *auth.Credential) []string {
	if c == nil {
		return nil
	}
	return []string{c.Key, c.Access, c.Refresh, c.OAuthClientSecret}
}
func safeProviderText(value string, c *auth.Credential) string {
	for _, secret := range providerSecretValues(c) {
		if secret != "" && strings.Contains(value, secret) {
			return "[redacted]"
		}
	}
	if len(value) > 2048 || strings.ContainsAny(value, "\x00\r\n") {
		return "[unavailable]"
	}
	return value
}
func (b *nativeProviderBackend) rows() ([]modelcatalog.NativeProviderRow, error) {
	settings, err := b.settings.Load()
	if err != nil {
		return nil, err
	}
	store, err := auth.New(b.stateDir)
	if err != nil {
		return nil, err
	}
	rows := make([]modelcatalog.NativeProviderRow, 0, len(model.Names()))
	for _, id := range model.Names() {
		p, ok := supportedNativeProvider(id)
		if !ok {
			continue
		}
		credential, found, err := store.Read(id)
		if err != nil {
			return nil, err
		}
		var c *auth.Credential
		if found {
			c = &credential
		}
		selected := settings
		selected.Defaults.Provider = id
		selected.Defaults.Model = model.DefaultModel(p)
		if settings.Defaults.Provider == id {
			selected.Defaults.Model = settings.Defaults.Model
		}
		status, err := workModelStatusForSettings(selected, b.stateDir)
		if err != nil {
			return nil, err
		}
		row := modelcatalog.NativeProviderRow{ID: id, Model: safeProviderText(status.Model, c), Stored: found, Ambient: len(remainingCredentialSources(p)) > 0, Fields: modelcatalog.NativeCloudFields(p), APIVersion: "v1", Options: map[string]string{}, Destination: safeProviderText(status.Processing.Destination, c)}
		if found {
			row.Kind = credentialKindLabel(credential)
			row.Expired = credential.Expired(time.Now())
		}
		row.Configured = found || row.Ambient || settings.Defaults.Provider == id || settings.ProviderEndpoint(id) != "" || len(settings.OptionsForProvider(id)) > 0
		row.Ready = status.Issue == "" && status.Provider != "" && row.Configured
		if status.Issue != "" {
			row.Issue = "Local configuration is incomplete. Check credentials, model and required endpoint/options."
		}
		for _, name := range row.Fields {
			if name == "api_version" {
				if u, err := url.Parse(settings.ProviderEndpoint(id)); err == nil && u.Query().Get("api-version") != "" {
					row.APIVersion = safeProviderText(u.Query().Get("api-version"), c)
				}
				continue
			}
			value := settings.OptionsForProvider(id)[name]
			if name == "gateway" && !safeProviderURL(value, false) {
				value = ""
			}
			row.Options[name] = safeProviderText(value, c)
		}
		rows = append(rows, row)
	}
	return rows, nil
}

// Apply never returns raw callback errors or credential-bearing objects. The
// review's expected credential/settings remain host-local and are single-use.
func (b *nativeProviderBackend) Apply(a modelcatalog.NativeProviderAction) modelcatalog.NativeProviderResult {
	b.mu.Lock()
	defer b.mu.Unlock()
	r := modelcatalog.NativeProviderResult{RequestID: a.RequestID, Action: a.Action, Provider: ""}
	fail := func(message string) modelcatalog.NativeProviderResult { r.Error = message; return r }
	if a.Action != "save" && a.APIKey != "" {
		return fail("Secret input is accepted only by an explicit reviewed Save.")
	}
	if a.Action == "status" {
		rows, err := b.rows()
		if err != nil {
			return fail("Provider status could not be read. Repair private storage and refresh.")
		}
		b.review = nil
		r.Rows = rows
		return r
	}
	if a.Action == "cancel" {
		b.review = nil
		return r
	}
	provider, ok := supportedNativeProvider(a.Provider)
	if !ok {
		return fail("This provider has no supported built-in API-key setup. Vendor CLI/OAuth, custom definitions and local runtime management are not available here.")
	}
	r.Provider = a.Provider
	if a.Action == "review" || a.Action == "review_remove" {
		b.review = nil
		settings, err := b.settings.Load()
		if err != nil {
			return fail("Configuration could not be read. No credential was changed.")
		}
		credentials, err := auth.New(b.stateDir)
		if err != nil {
			return fail("Private credentials could not be opened.")
		}
		current, found, err := credentials.Read(a.Provider)
		if err != nil {
			return fail("Private credentials could not be read.")
		}
		review := &nativeProviderReview{settings: settings, replace: a.ReplaceKey, expires: time.Now().Add(5 * time.Minute)}
		if found {
			review.credential = &current
		}
		review.public = modelcatalog.NativeProviderReview{ID: providerNonce(), Provider: a.Provider, Operation: "save", CredentialAction: "retain", SetsDefaults: true}
		if review.public.ID == "" {
			return fail("A secure review identity could not be created.")
		}
		if a.Action == "review_remove" {
			if !found {
				return fail("No Gator credential is stored for this provider. Environment and vendor credentials are unchanged.")
			}
			review.public.Operation = "remove"
			review.public.CredentialAction = "remove Gator-stored " + credentialKindLabel(current)
			review.public.SetsDefaults = false
		} else {
			if a.Input == nil {
				return fail("Configuration fields are required.")
			}
			input := a.Input
			endpoint := settings.ProviderEndpoint(a.Provider)
			switch input.EndpointMode {
			case "retain":
			case "default":
				endpoint = ""
			case "override":
				endpoint = input.Endpoint
			default:
				return fail("Choose retain, provider default or an explicit endpoint override.")
			}
			azure := provider == model.AzureOpenAI || provider == model.AzureOpenAIResponses
			if !safeProviderURL(endpoint, azure) {
				return fail("Endpoint must be an absolute HTTP(S) URL without credentials, fragments or secret query parameters. Choose an explicit safe replacement; no settings changed.")
			}
			for _, v := range input.Options {
				if len(v) > 2048 {
					return fail("Provider option is too long.")
				}
			}
			if gateway := input.Options["gateway"]; gateway != "" && !safeProviderURL(gateway, false) {
				return fail("Gateway URL cannot contain credentials or query parameters.")
			}
			setup, err := modelcatalog.NormalizeNativeCloudSetup(modelcatalog.CloudModelSetup{Provider: a.Provider, Model: input.Model, BaseURL: endpoint, Options: input.Options}, input.APIVersion)
			if err != nil {
				return fail("Invalid provider configuration. Check the model/deployment, endpoint, API version and provider-specific fields. Nothing was saved.")
			}
			// A retained credential must not be projected through another input field.
			for _, value := range append([]string{setup.Model, setup.BaseURL}, providerOptionValues(setup.Options)...) {
				if safeProviderText(value, review.credential) != value {
					return fail("Configuration fields cannot contain stored credential material.")
				}
			}
			review.setup = setup
			review.public.Options = cloneTUIProviderOptions(setup.Options)
			if azure {
				u, _ := url.Parse(setup.BaseURL)
				review.public.Options["api_version"] = u.Query().Get("api-version")
			}
			review.public.Model = setup.Model
			review.public.EndpointMode = input.EndpointMode
			review.public.Destination = workModelProcessing(a.Provider, setup.BaseURL).Destination
			if a.ReplaceKey {
				review.public.CredentialAction = "add API key"
				if found {
					review.public.CredentialAction = "replace Gator-stored " + credentialKindLabel(current) + " with API key"
				}
			}
		}
		b.review = review
		r.Review = &review.public
		return r
	}
	if a.Action != "save" && a.Action != "remove" {
		return fail("Unsupported provider operation.")
	}
	review := b.review
	b.review = nil
	if review == nil || review.public.ID != a.ReviewID || review.public.Provider != a.Provider || time.Now().After(review.expires) || review.public.Operation != map[string]string{"save": "save", "remove": "remove"}[a.Action] {
		return fail("This review expired or changed. Refresh and explicitly review again; nothing was retried.")
	}
	var err error
	if a.Action == "remove" {
		backend := &tuiModelManagementBackend{settings: b.settings, stateDir: b.stateDir}
		_, err = backend.removeReviewedCredential(a.Provider, review.credential)
	} else {
		if review.replace != (strings.TrimSpace(a.APIKey) != "") {
			return fail("The reviewed credential intent changed. Re-enter the key and review again.")
		}
		if len(a.APIKey) > 16*1024 || strings.ContainsAny(a.APIKey, "\x00\r\n") {
			return fail("The API key format is invalid. Re-enter it; nothing was saved.")
		}
		if a.APIKey != "" {
			for _, value := range append([]string{review.setup.Model, review.setup.BaseURL}, providerOptionValues(review.setup.Options)...) {
				if strings.Contains(value, strings.TrimSpace(a.APIKey)) {
					return fail("Credential material cannot be saved in configuration fields.")
				}
			}
		}
		setup := review.setup
		setup.APIKey = a.APIKey
		err = saveTUICloudModelConfiguration(reviewedCloudSettings{store: b.settings, expected: review.settings}, b.stateDir, cloudCredentialReview{provider: a.Provider, expected: review.credential})(setup)
		setup.APIKey = ""
	}
	a.APIKey = ""
	if err != nil {
		r.Conflict = errors.Is(err, auth.ErrCredentialChanged) || errors.Is(err, errProviderReviewChanged)
		if r.Conflict {
			return fail("Credential or configuration changed after review. Refresh, inspect and explicitly review again; concurrent changes were preserved.")
		}
		return fail("Provider changes were not confirmed. Check private storage, refresh status and review again. Credential rollback only applies to this operation's own write; nothing is retried automatically.")
	}
	r.Confirmed = true
	r.Changed = true
	if a.Action == "remove" {
		r.Notice = "Removed only this Gator-stored credential. Models/endpoints are unchanged. Environment or vendor CLI credentials may still authenticate future requests."
	} else {
		r.Notice = "Saved Gator configuration and future default model. Local readiness is not live verification; no inference request was sent."
	}
	r.Rows, err = b.rows()
	if err != nil {
		r.Notice += " Status refresh failed; refresh explicitly before using the provider."
	}
	return r
}
func providerOptionValues(values map[string]string) []string {
	r := make([]string, 0, len(values))
	for _, v := range values {
		r = append(r, v)
	}
	return r
}
