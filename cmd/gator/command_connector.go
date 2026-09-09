package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/gongahkia/gator/internal/action"
	"github.com/gongahkia/gator/internal/auth"
	"github.com/gongahkia/gator/internal/config"
	"github.com/gongahkia/gator/internal/connector"
)

const maxConnectorTokenBytes = 64 * 1024

var connectorEnvironmentPattern = regexp.MustCompile(`\A[A-Z_][A-Z0-9_]{0,127}\z`)

func connectorCommand(arguments []string, out io.Writer) error {
	return connectorCommandWithIO(arguments, os.Stdin, out)
}

func connectorCommandWithIO(arguments []string, in io.Reader, out io.Writer) error {
	if len(arguments) == 0 || arguments[0] == "list" {
		return listConnectors(out)
	}
	actionName := arguments[0]
	if len(arguments) < 2 {
		return fmt.Errorf("connector %s requires a connector ID", actionName)
	}
	id := arguments[1]
	switch actionName {
	case "add":
		return addConnector(id, arguments[2:], out)
	case "status":
		if len(arguments) != 2 {
			return errors.New("usage: gator connector status ID")
		}
		return connectorStatus(id, out)
	case "login":
		return loginConnector(id, arguments[2:], in, out)
	case "logout":
		if len(arguments) != 2 {
			return errors.New("usage: gator connector logout ID")
		}
		return logoutConnector(id, out)
	case "test":
		if len(arguments) != 2 {
			return errors.New("usage: gator connector test ID")
		}
		return testConnector(id, out)
	case "remove":
		return removeConnector(id, arguments[2:], out)
	case "permission":
		return setConnectorPermission(id, arguments[2:], out)
	default:
		return fmt.Errorf("unknown connector action %q", actionName)
	}
}

func listConnectors(out io.Writer) error {
	settings, err := loadSettings()
	if err != nil {
		return err
	}
	registry, err := connector.NewRegistry(settings.Connectors)
	if err != nil {
		return err
	}
	credentials, err := gatorCredentials()
	if err != nil {
		return err
	}
	if _, err := fmt.Fprintln(out, "Connectors:"); err != nil {
		return err
	}
	if len(registry.List()) == 0 {
		_, err := fmt.Fprintln(out, "  (none configured)")
		return err
	}
	for _, descriptor := range registry.List() {
		status, err := connectorAuthStatus(descriptor, credentials)
		if err != nil {
			return err
		}
		if _, err := fmt.Fprintf(out, "  %s  %s  %s  %s\n", descriptor.ID, descriptor.Name, descriptor.Kind, status); err != nil {
			return err
		}
	}
	return nil
}

func addConnector(id string, arguments []string, out io.Writer) error {
	flags := flag.NewFlagSet("connector add", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	name := flags.String("name", id, "display name")
	kindName := flags.String("kind", "json", "connector kind: json, webhook, slack, google, atlassian, notion, or mcp")
	resource := flags.String("url", "", "exact resource URL")
	authentication := flags.String("auth", "", "authentication: none, bearer, or oauth")
	searchTool := flags.String("search-tool", "", "remote MCP tool mapped to search")
	readTool := flags.String("read-tool", "", "remote MCP tool mapped to read")
	actionTool := flags.String("action-tool", "", "remote MCP tool mapped to an approved action")
	oauthClientID := flags.String("oauth-client-id", "", "public OAuth app client ID")
	oauthAuthorizeURL := flags.String("oauth-authorize-url", "", "OAuth authorization endpoint")
	oauthTokenURL := flags.String("oauth-token-url", "", "OAuth token endpoint")
	oauthRedirectURL := flags.String("oauth-redirect-url", "", "registered HTTP loopback callback URL")
	oauthScopes := flags.String("oauth-scopes", "", "space-separated OAuth scopes")
	if err := flags.Parse(arguments); err != nil {
		return err
	}
	if len(flags.Args()) != 0 {
		return errors.New("usage: gator connector add ID --kind json|webhook|slack|google|atlassian|notion|mcp [--url URL] [--auth none|bearer|oauth]")
	}
	kind := ""
	switch strings.ToLower(strings.TrimSpace(*kindName)) {
	case "json", "http-json", connector.KindHTTPJSON:
		kind = connector.KindHTTPJSON
	case "webhook", connector.KindHTTPWebhook:
		kind = connector.KindHTTPWebhook
	case connector.KindSlack:
		kind = connector.KindSlack
	case "google", "google-workspace", connector.KindGoogle:
		kind = connector.KindGoogle
	case connector.KindAtlassian, "jira":
		kind = connector.KindAtlassian
	case connector.KindNotion:
		kind = connector.KindNotion
	case "mcp", connector.KindRemoteMCP:
		kind = connector.KindRemoteMCP
	default:
		return fmt.Errorf("unknown connector kind %q", *kindName)
	}
	if strings.TrimSpace(*resource) == "" {
		switch kind {
		case connector.KindSlack:
			*resource = "https://slack.com/api"
		case connector.KindGoogle:
			*resource = "https://www.googleapis.com"
		case connector.KindNotion:
			*resource = "https://api.notion.com/v1"
		}
	}
	if strings.TrimSpace(*authentication) == "" {
		*authentication = connector.AuthNone
		if kind != connector.KindHTTPJSON && kind != connector.KindHTTPWebhook {
			*authentication = connector.AuthBearer
		}
	}
	descriptor := connector.Descriptor{
		Version: connector.DescriptorVersion, ID: id, Name: strings.TrimSpace(*name),
		Kind: kind, Resource: strings.TrimSpace(*resource), Authentication: strings.TrimSpace(*authentication),
		SearchTool: strings.TrimSpace(*searchTool), ReadTool: strings.TrimSpace(*readTool), ActionTool: strings.TrimSpace(*actionTool),
		OAuthClientID: strings.TrimSpace(*oauthClientID), OAuthAuthorizeURL: strings.TrimSpace(*oauthAuthorizeURL), OAuthTokenURL: strings.TrimSpace(*oauthTokenURL), OAuthRedirectURL: strings.TrimSpace(*oauthRedirectURL), OAuthScopes: strings.TrimSpace(*oauthScopes),
	}
	if err := descriptor.Validate(); err != nil {
		return err
	}
	store, settings, err := connectorSettings()
	if err != nil {
		return err
	}
	for _, existing := range settings.Connectors {
		if existing.ID == descriptor.ID {
			return fmt.Errorf("connector %q is already configured; remove it before changing its resource", descriptor.ID)
		}
	}
	settings.Connectors = append(settings.Connectors, descriptor)
	sort.Slice(settings.Connectors, func(left, right int) bool { return settings.Connectors[left].ID < settings.Connectors[right].ID })
	if err := store.Save(settings); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(out, "Added connector %s (%s).\n", descriptor.ID, descriptor.Resource); err != nil {
		return err
	}
	if descriptor.Authentication == connector.AuthBearer {
		_, err = fmt.Fprintf(out, "Authenticate it with: gator connector login %s --from-env TOKEN_ENV\n", descriptor.ID)
		return err
	}
	if descriptor.Authentication == connector.AuthOAuth {
		_, err = fmt.Fprintf(out, "Authenticate the BYO public OAuth app with: gator connector login %s\n", descriptor.ID)
		return err
	}
	return nil
}

func connectorStatus(id string, out io.Writer) error {
	descriptor, credentials, err := configuredConnector(id)
	if err != nil {
		return err
	}
	status, err := connectorAuthStatus(descriptor, credentials)
	if err != nil {
		return err
	}
	if _, err = fmt.Fprintf(out, "Connector %s\n  name: %s\n  kind: %s\n  resource: %s\n  authentication: %s\n  operations:\n", descriptor.ID, descriptor.Name, descriptor.Kind, descriptor.Resource, status); err != nil {
		return err
	}
	for _, operation := range descriptor.Operations() {
		if _, err := fmt.Fprintf(out, "    %s: %s\n", operation.ID, operation.Capability); err != nil {
			return err
		}
	}
	return nil
}

func loginConnector(id string, arguments []string, in io.Reader, out io.Writer) error {
	flags := flag.NewFlagSet("connector login", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	fromEnvironment := flags.String("from-env", "", "read bearer token from an environment variable")
	fromStdin := flags.Bool("token-stdin", false, "read bearer token from standard input")
	if err := flags.Parse(arguments); err != nil {
		return err
	}
	descriptor, credentials, err := configuredConnector(id)
	if err != nil {
		return err
	}
	credentialSources := 0
	if *fromEnvironment != "" {
		credentialSources++
	}
	if *fromStdin {
		credentialSources++
	}
	if len(flags.Args()) != 0 {
		return errors.New("connector login received unexpected arguments")
	}
	if descriptor.Authentication == connector.AuthOAuth {
		if credentialSources != 0 {
			return errors.New("OAuth connector login does not accept bearer-token flags")
		}
		return loginConnectorOAuth(descriptor, credentials, out)
	}
	if descriptor.Authentication != connector.AuthBearer {
		return fmt.Errorf("connector %q is configured without bearer authentication", id)
	}
	if credentialSources != 1 {
		return errors.New("bearer connector login requires exactly one of --from-env NAME or --token-stdin")
	}
	token := ""
	if *fromEnvironment != "" {
		if !connectorEnvironmentPattern.MatchString(*fromEnvironment) {
			return errors.New("connector token environment variable name is invalid")
		}
		token = os.Getenv(*fromEnvironment)
		if token == "" {
			return fmt.Errorf("environment variable %s is empty", *fromEnvironment)
		}
	} else {
		contents, err := io.ReadAll(io.LimitReader(in, maxConnectorTokenBytes+1))
		if err != nil {
			return fmt.Errorf("read connector token: %w", err)
		}
		if len(contents) > maxConnectorTokenBytes {
			return errors.New("connector token exceeds 64 KiB")
		}
		token = string(contents)
	}
	credential, err := auth.NewBearerToken(token, time.Time{})
	if err != nil {
		return err
	}
	if err := credentials.Put(descriptor.CredentialRef(), credential); err != nil {
		return err
	}
	_, err = fmt.Fprintf(out, "Stored a resource-bound bearer credential for connector %s.\n", id)
	return err
}

func loginConnectorOAuth(descriptor connector.Descriptor, credentials auth.Store, out io.Writer) error {
	flow := auth.BrowserFlow{
		ClientID: descriptor.OAuthClientID, AuthorizationURL: descriptor.OAuthAuthorizeURL,
		TokenURL: descriptor.OAuthTokenURL, RedirectURL: descriptor.OAuthRedirectURL,
		Scopes: strings.Fields(descriptor.OAuthScopes), AllowMissingExpiry: true, RequireBearerToken: true,
	}
	attempt, err := auth.BeginBrowserFlow(flow)
	if err != nil {
		return fmt.Errorf("start connector OAuth login: %w", err)
	}
	callback, err := attempt.StartCallback()
	if err != nil {
		return fmt.Errorf("start connector OAuth callback: %w", err)
	}
	defer callback.Close()
	if _, err := fmt.Fprintf(out, "Open this URL to authenticate connector %s with your OAuth app:\n%s\n\nWaiting for the local callback...\n", descriptor.ID, attempt.AuthorizationURL()); err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	code, err := callback.Wait(ctx)
	if err != nil {
		return fmt.Errorf("complete connector OAuth login: %w", err)
	}
	credential, err := attempt.Exchange(ctx, code)
	if err != nil {
		return fmt.Errorf("exchange connector OAuth credential: %w", err)
	}
	if err := credentials.Put(descriptor.CredentialRef(), credential); err != nil {
		return fmt.Errorf("store connector OAuth credential: %w", err)
	}
	_, err = fmt.Fprintf(out, "Stored a resource-bound OAuth credential for connector %s.\n", descriptor.ID)
	return err
}

func logoutConnector(id string, out io.Writer) error {
	descriptor, credentials, err := configuredConnector(id)
	if err != nil {
		return err
	}
	if err := credentials.Delete(descriptor.CredentialRef()); err != nil {
		return err
	}
	_, err = fmt.Fprintf(out, "Removed the local credential for connector %s.\n", id)
	return err
}

func testConnector(id string, out io.Writer) error {
	settings, err := loadSettings()
	if err != nil {
		return err
	}
	registry, err := connector.NewRegistry(settings.Connectors)
	if err != nil {
		return err
	}
	descriptor, found := registry.Get(id)
	if !found {
		return fmt.Errorf("connector %q is not configured", id)
	}
	if descriptor.Kind == connector.KindHTTPWebhook {
		return fmt.Errorf("connector %q is an action endpoint; test it through a --actions draft work run, which does not send", id)
	}
	credentials, err := gatorCredentials()
	if err != nil {
		return err
	}
	operation, input := connectorTestInvocation(descriptor)
	result, err := (connector.Runtime{Registry: registry, Credentials: credentials}).Invoke(context.Background(), action.Inspect, id, operation, input)
	if err != nil {
		return err
	}
	_, err = fmt.Fprintf(out, "Connector %s is reachable: %d bytes, sha256:%s\n", id, result.Provenance.Bytes, result.Provenance.SHA256[:12])
	return err
}

func connectorTestInvocation(descriptor connector.Descriptor) (string, []byte) {
	switch descriptor.Kind {
	case connector.KindSlack:
		return "whoami", []byte(`{}`)
	case connector.KindGoogle:
		return "drive_search", []byte(`{}`)
	case connector.KindAtlassian:
		return "jira_search", []byte(`{}`)
	case connector.KindNotion:
		return "search", []byte(`{}`)
	case connector.KindRemoteMCP:
		operation := "search"
		if descriptor.SearchTool == "" {
			operation = "read"
		}
		return operation, []byte(`{"arguments":{}}`)
	default:
		return "fetch", []byte(`{}`)
	}
}

func setConnectorPermission(id string, arguments []string, out io.Writer) error {
	if len(arguments) != 3 || (arguments[1] != "read" && arguments[1] != "write") {
		return errors.New("usage: gator connector permission ID OPERATION read|write allow|ask|deny|draft")
	}
	descriptor, _, err := configuredConnector(id)
	if err != nil {
		return err
	}
	operation := arguments[0]
	found := false
	for _, candidate := range descriptor.Operations() {
		found = found || candidate.ID == operation
	}
	if !found {
		return fmt.Errorf("connector %q has no operation %q", id, operation)
	}
	permission := connector.Permission(arguments[2])
	rule := connector.PermissionRule{ConnectorID: id, Operation: operation}
	if arguments[1] == "read" {
		rule.Read = permission
	} else {
		rule.Write = permission
	}
	if err := rule.Validate(); err != nil {
		return err
	}
	store, settings, err := connectorSettings()
	if err != nil {
		return err
	}
	filtered := settings.ConnectorPermissions[:0]
	for _, existing := range settings.ConnectorPermissions {
		replaceRead := arguments[1] == "read" && existing.Read != ""
		replaceWrite := arguments[1] == "write" && existing.Write != ""
		if existing.ConnectorID == id && existing.Operation == operation && (replaceRead || replaceWrite) {
			continue
		}
		filtered = append(filtered, existing)
	}
	settings.ConnectorPermissions = append(filtered, rule)
	if err := store.Save(settings); err != nil {
		return err
	}
	_, err = fmt.Fprintf(out, "Set %s/%s %s permission to %s.\n", id, operation, arguments[1], permission)
	return err
}

func removeConnector(id string, arguments []string, out io.Writer) error {
	if len(arguments) != 1 || arguments[0] != "--yes" {
		return errors.New("connector remove requires --yes")
	}
	store, settings, err := connectorSettings()
	if err != nil {
		return err
	}
	index := -1
	var descriptor connector.Descriptor
	for candidate, existing := range settings.Connectors {
		if existing.ID == id {
			index = candidate
			descriptor = existing
			break
		}
	}
	if index < 0 {
		return fmt.Errorf("connector %q is not configured", id)
	}
	settings.Connectors = append(settings.Connectors[:index], settings.Connectors[index+1:]...)
	if err := store.Save(settings); err != nil {
		return err
	}
	credentials, err := gatorCredentials()
	if err != nil {
		return err
	}
	if err := credentials.Delete(descriptor.CredentialRef()); err != nil {
		return err
	}
	_, err = fmt.Fprintf(out, "Removed connector %s and its local credential. This cannot be undone from Gator.\n", id)
	return err
}

func connectorSettings() (config.Store, config.Settings, error) {
	store, err := config.DefaultStore()
	if err != nil {
		return config.Store{}, config.Settings{}, err
	}
	settings, err := store.Load()
	return store, settings, err
}

func configuredConnector(id string) (connector.Descriptor, auth.Store, error) {
	settings, err := loadSettings()
	if err != nil {
		return connector.Descriptor{}, auth.Store{}, err
	}
	registry, err := connector.NewRegistry(settings.Connectors)
	if err != nil {
		return connector.Descriptor{}, auth.Store{}, err
	}
	descriptor, found := registry.Get(id)
	if !found {
		return connector.Descriptor{}, auth.Store{}, fmt.Errorf("connector %q is not configured", id)
	}
	credentials, err := gatorCredentials()
	return descriptor, credentials, err
}

func connectorAuthStatus(descriptor connector.Descriptor, credentials auth.Store) (string, error) {
	if descriptor.Authentication == connector.AuthNone {
		return "no authentication", nil
	}
	credential, found, err := credentials.Read(descriptor.CredentialRef())
	if err != nil {
		return "", err
	}
	if !found {
		return "login required", nil
	}
	if credential.Expired(time.Now()) {
		return "credential expired", nil
	}
	return "authenticated", nil
}
