package main

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/gongahkia/gator/internal/action"
	"github.com/gongahkia/gator/internal/auth"
	"github.com/gongahkia/gator/internal/config"
	"github.com/gongahkia/gator/internal/connector"
	"github.com/gongahkia/gator/internal/state"
	"github.com/gongahkia/gator/internal/uisnapshot"
	"github.com/gongahkia/gator/internal/workapp"
)

type connectionControls struct {
	stateDir string
	checkMu  sync.Mutex
	checks   map[string]sourceReadCheck
	signIn   connectionSignIn
}

func credentialReview(c *auth.Credential) string {
	data, _ := json.Marshal(c)
	return fmt.Sprintf("%x", sha256.Sum256(data))
}
func connectionCredential(s auth.Store, d connector.Descriptor) (*auth.Credential, error) {
	c, found, err := s.Read(d.CredentialRef())
	if err != nil || !found {
		return nil, err
	}
	return &c, nil
}

func (c *connectionControls) call(ctx context.Context, operation string, args []string) (workapp.ControlScreen, error) {
	// Browser sign-in keeps its own shared profile lease until it finishes.
	release, err := state.Guard(c.stateDir, false)
	if err != nil {
		return workapp.ControlScreen{}, err
	}
	defer release()
	id, notice := "", ""
	if len(args) > 0 {
		id = args[0]
	}
	switch operation {
	case "status", "select":
	case "add-google", "add-google-bundled":
		if operation == "add-google-bundled" {
			if !bundledGoogleAvailable() || len(args) != 2 {
				return workapp.ControlScreen{}, errors.New("this build has no configured Google sign-in; use your own desktop OAuth client")
			}
			args = append(append([]string(nil), args...), googleDesktopClientID)
		}
		if len(args) != 3 {
			return workapp.ControlScreen{}, errors.New("provide a connection ID, display name and desktop OAuth client ID")
		}
		d := connector.Descriptor{Version: connector.DescriptorVersion, ID: strings.TrimSpace(args[0]), Name: strings.TrimSpace(args[1]), Kind: connector.KindGoogle, Resource: "https://www.googleapis.com", Authentication: connector.AuthOAuth, OAuthClientID: strings.TrimSpace(args[2]), OAuthAuthorizeURL: connector.GoogleOAuthAuthorizeURL, OAuthTokenURL: connector.GoogleOAuthTokenURL, OAuthRedirectURL: connector.GoogleOAuthRedirectURL, OAuthScopes: "openid https://www.googleapis.com/auth/userinfo.email " + connector.GoogleDailyOAuthScopes}
		if err := addNativeConnection(d); err != nil {
			return workapp.ControlScreen{}, err
		}
		id, notice = d.ID, "Connection added. Sign in to give it access."
	case "add-token":
		if len(args) != 4 {
			return workapp.ControlScreen{}, errors.New("provide a connection ID, display name, service and resource URL")
		}
		d := connector.Descriptor{Version: connector.DescriptorVersion, ID: strings.TrimSpace(args[0]), Name: strings.TrimSpace(args[1]), Kind: strings.TrimSpace(args[2]), Resource: strings.TrimSpace(args[3]), Authentication: connector.AuthBearer}
		if !slices.Contains([]string{connector.KindHTTPJSON, connector.KindSlack, connector.KindNotion, connector.KindAtlassian}, d.Kind) {
			return workapp.ControlScreen{}, errors.New("choose http_json, slack, notion or atlassian")
		}
		if err := addNativeConnection(d); err != nil {
			return workapp.ControlScreen{}, err
		}
		id, notice = d.ID, "Connection added. Add its access token to continue."
	case "signin":
		if len(args) != 3 {
			return workapp.ControlScreen{}, errors.New("review the connection before signing in")
		}
		if _, err := c.signIn.Begin(c.stateDir, id, args[1], args[2]); err != nil {
			return workapp.ControlScreen{}, err
		}
	case "enable-email-sending":
		if len(args) != 4 {
			return workapp.ControlScreen{}, errors.New("review the connection and requested sending permission again")
		}
		if _, err := c.signIn.EnableGmailSending(c.stateDir, id, args[1], args[2], args[3]); err != nil {
			return workapp.ControlScreen{}, err
		}
	case "disable-email-sending":
		if len(args) != 3 {
			return workapp.ControlScreen{}, errors.New("review the connection before changing sending permission")
		}
		d, credentials, err := configuredConnector(id)
		if err != nil {
			return workapp.ControlScreen{}, err
		}
		if connectorLoginReview(d) != args[1] {
			return workapp.ControlScreen{}, errors.New("connection changed; review its permissions again")
		}
		prior, err := connectionCredential(credentials, d)
		if err != nil {
			return workapp.ControlScreen{}, err
		}
		if credentialReview(prior) != args[2] {
			return workapp.ControlScreen{}, auth.ErrCredentialChanged
		}
		next, err := gmailSendingDescriptor(d, false)
		if err != nil {
			return workapp.ControlScreen{}, err
		}
		c.signIn.Cancel(id)
		store, err := config.DefaultStore()
		if err != nil {
			return workapp.ControlScreen{}, err
		}
		if err := replaceReviewedConnection(store, d, next); err != nil {
			return workapp.ControlScreen{}, err
		}
		notice = "Email sending turned off for new tasks. Active tasks keep their reviewed connection until they finish. To revoke Google's grant, use Google account settings."
	case "cancel-signin":
		c.signIn.Cancel(id)
		notice = "Sign-in closed. No further authorization will be accepted."
	case "token", "disconnect", "remove":
		if len(args) < 3 || operation == "token" && len(args) != 4 {
			return workapp.ControlScreen{}, errors.New("review this connection again")
		}
		d, credentials, err := configuredConnector(id)
		if err != nil {
			return workapp.ControlScreen{}, err
		}
		if connectorLoginReview(d) != args[1] {
			return workapp.ControlScreen{}, errors.New("connection changed; review it again")
		}
		prior, err := connectionCredential(credentials, d)
		if err != nil {
			return workapp.ControlScreen{}, err
		}
		if credentialReview(prior) != args[2] {
			return workapp.ControlScreen{}, auth.ErrCredentialChanged
		}
		var next *auth.Credential
		if operation == "token" {
			if d.Authentication != connector.AuthBearer {
				return workapp.ControlScreen{}, errors.New("this connection needs browser sign-in")
			}
			token, err := auth.NewBearerToken(args[3], time.Time{})
			if err != nil {
				return workapp.ControlScreen{}, err
			}
			next = &token
		}
		if err := credentials.ReplaceReviewed(d.CredentialRef(), prior, next); err != nil {
			return workapp.ControlScreen{}, err
		}
		if operation == "token" {
			notice = "Token saved. Check access to verify it."
		} else {
			c.signIn.Cancel(id)
			notice = "Disconnected on this device. To revoke access at the service, use its account settings."
		}
		if operation == "remove" {
			store, err := config.DefaultStore()
			if err != nil {
				return workapp.ControlScreen{}, err
			}
			err = store.Update(func(settings *config.Settings) error {
				for i, current := range settings.Connectors {
					if current.ID != id {
						continue
					}
					if connectorLoginReview(current) != args[1] {
						return errors.New("connection changed; credentials disconnected, but configuration kept")
					}
					settings.Connectors = append(settings.Connectors[:i], settings.Connectors[i+1:]...)
					settings.ConnectorPermissions = slices.DeleteFunc(settings.ConnectorPermissions, func(rule connector.PermissionRule) bool { return rule.ConnectorID == id })
					return nil
				}
				return nil
			})
			if err != nil {
				return workapp.ControlScreen{}, err
			}
			id, notice = "", "Connection removed. Follow-ups using it need their sources reviewed."
		}
	case "verify":
		if len(args) != 2 {
			return workapp.ControlScreen{}, errors.New("choose a connection to check")
		}
		d, credentials, err := configuredConnector(id)
		if err != nil {
			return workapp.ControlScreen{}, err
		}
		if connectorLoginReview(d) != args[1] {
			return workapp.ControlScreen{}, errors.New("connection changed; refresh before checking access")
		}
		credential, credentialErr := connectionCredential(credentials, d)
		if credentialErr != nil {
			return workapp.ControlScreen{}, credentialErr
		}
		checkedSuccessfully := false
		defer func() {
			c.recordSourceCheck(id, connectorLoginReview(d)+credentialReview(credential), checkedSuccessfully)
		}()
		registry, err := connector.NewRegistry([]connector.Descriptor{d})
		if err != nil {
			return workapp.ControlScreen{}, err
		}
		ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
		defer cancel()
		runtime := connector.Runtime{Registry: registry, Credentials: credentials}
		account := ""
		if d.Kind == connector.KindGoogle && d.Authentication == connector.AuthOAuth {
			identity, err := runtime.GoogleAccount(ctx, id)
			if err != nil {
				return workapp.ControlScreen{}, fmt.Errorf("could not verify account: %w", err)
			}
			account = identity.Email
			runtime.OAuthAccountPins = map[string]string{id: identity.Pin}
		}
		if d.Kind == connector.KindHTTPWebhook {
			return workapp.ControlScreen{}, errors.New("this connection only sends actions; check it by reviewing a draft in chat")
		}
		probes := connectionReadProbes(d, time.Now())
		var checked []string
		for _, probe := range probes {
			if _, err := runtime.Invoke(ctx, action.Inspect, id, probe.operation, probe.input); err != nil {
				return workapp.ControlScreen{}, fmt.Errorf("%s read check failed: %w", probe.label, err)
			}
			checked = append(checked, probe.label)
		}
		checkedSuccessfully = len(probes) > 0
		notice = "Read access verified just now."
		if len(checked) > 0 {
			notice += " Checked: " + strings.Join(checked, ", ") + "."
		} else {
			notice = "Account identity verified. No supported read permissions are configured."
		}
		if account != "" {
			notice = "Account " + account + " · " + notice
		}

	default:
		return workapp.ControlScreen{}, errors.New("unknown connection action")
	}
	screen, err := c.screen(id)
	if notice != "" {
		screen.Panel.Notice = notice
	}
	return screen, err
}

func addNativeConnection(d connector.Descriptor) error {
	if err := d.Validate(); err != nil {
		return err
	}
	store, err := config.DefaultStore()
	if err != nil {
		return err
	}
	return store.Update(func(s *config.Settings) error {
		for _, existing := range s.Connectors {
			if existing.ID == d.ID {
				return errors.New("that connection ID already exists; choose another")
			}
		}
		s.Connectors = append(s.Connectors, d)
		return nil
	})
}

func (c *connectionControls) screen(id string) (workapp.ControlScreen, error) {
	screen := workapp.ControlScreen{Location: id, Panel: uisnapshot.Panel{Kind: "connections", Title: "Connections", NoticeTone: "info", Subtitle: "Bring your accounts into the work you choose."}}
	status := c.signIn.Status()
	if id == "" || id == "@advanced" {
		_, settings, err := connectorSettings()
		if err != nil {
			return screen, err
		}
		credentials, err := gatorCredentials()
		if err != nil {
			return screen, err
		}
		for _, d := range settings.Connectors {
			detail, err := nativeCredentialStatus(d, credentials)
			if err != nil {
				detail = "Credential status unavailable"
			}
			screen.Actions = append(screen.Actions, workapp.ControlAction{Label: d.Name, Detail: detail, Action: "select", Args: []string{d.ID}})
		}
		if status.ID != "" && !status.Done {
			screen.Panel.Notice = "A browser sign-in is waiting. Open its connection to continue or cancel."
		}
		screen.Actions = append(screen.Actions,
			workapp.ControlAction{Key: "G", Label: "Connect Google email", Action: "add-google", Description: "Read email. This setup requires your own Google Cloud desktop OAuth client; Gator does not yet provide a managed Google sign-in app. You will sign in after saving, and choose access per chat or follow-up.", Review: true, Fields: []workapp.ControlField{{Label: "Connection ID", Value: "google", Help: "A unique lowercase name, for example google-personal."}, {Label: "Display name", Value: "Google"}, {Label: "Desktop OAuth client ID", Help: "From a Google Cloud OAuth client of type Desktop app. No client secret here."}}},
			workapp.ControlAction{Key: "T", Label: "Add a service token connection", Action: "add-token", Description: "For an existing Slack, Notion, Atlassian or JSON API account. Add the service and resource first, then enter its token privately.", Review: true, Fields: []workapp.ControlField{{Label: "Connection ID"}, {Label: "Display name"}, {Label: "Service", Kind: "select", Value: connector.KindHTTPJSON, Options: []uisnapshot.Option{{Value: connector.KindHTTPJSON, Label: "JSON API"}, {Value: connector.KindSlack, Label: "Slack"}, {Value: connector.KindNotion, Label: "Notion"}, {Value: connector.KindAtlassian, Label: "Atlassian"}}}, {Label: "Resource URL", Help: "The HTTPS API resource for this account. Do not include tokens in the URL."}}},
			workapp.ControlAction{Key: "R", Label: "Refresh", Action: "status"})
		if bundledGoogleAvailable() {
			for i := range screen.Actions {
				if screen.Actions[i].Action == "add-google" {
					screen.Actions[i].Key = "O"
					screen.Actions[i].Label = "Use your own Google OAuth client"
					screen.Actions[i].Description = "Advanced setup using your own Google Cloud Desktop app client. Sign in after saving; choose access per chat or follow-up."
				}
			}
			screen.Actions = append([]workapp.ControlAction{bundledGoogleAction()}, screen.Actions...)
		}
		screen.Panel.Blocks = []uisnapshot.Block{{Title: "Access stays scoped", Lines: []string{"Connecting an account makes it available. Choose it in chat settings or a follow-up agreement before Gator uses it.", "You can start with a message or attached file without connecting any accounts."}}}
		if id == "@advanced" {
			screen.Panel.Title = "Advanced connections"
			screen.Panel.BackLabel = "Back to connections"
			screen.Panel.Subtitle = "Bring your own service token and exact API resource. Existing access policies and action approvals still apply."
			actions := []workapp.ControlAction{}
			for _, a := range screen.Actions {
				if a.Action == "add-token" {
					actions = append(actions, a)
				}
			}
			screen.Actions = actions
		} else {
			actions := []workapp.ControlAction{}
			for _, a := range screen.Actions {
				if a.Action != "add-token" {
					actions = append(actions, a)
				}
			}
			screen.Actions = append(actions, workapp.ControlAction{Key: "T", Label: "Advanced connections", Detail: "Slack, Notion, Atlassian and JSON APIs · bring your own credentials", Action: "select", Args: []string{"@advanced"}})
		}
		return screen, nil
	}
	d, credentials, err := configuredConnector(id)
	if err != nil {
		return screen, err
	}
	prior, err := connectionCredential(credentials, d)
	if err != nil {
		return screen, err
	}
	localStatus, err := nativeCredentialStatus(d, credentials)
	if err != nil {
		return screen, err
	}
	screen.Panel.Title, screen.Panel.BackLabel = d.Name, "Back to connections"
	screen.Panel.Subtitle = localStatus
	screen.Panel.Summary = []uisnapshot.Field{{Label: "Service", Value: connectionServiceLabel(d.Kind)}, {Label: "Requested access", Value: connectionAccessLabel(d)}}
	details := []uisnapshot.Field{{Label: "Resource", Value: d.Resource}, {Label: "Authentication", Value: d.Authentication}, {Label: "Connection ID", Value: d.ID}}
	if d.OAuthScopes != "" {
		details = append(details, uisnapshot.Field{Label: "Requested permissions", Value: d.OAuthScopes})
	}
	screen.Panel.Blocks = []uisnapshot.Block{{Title: "Connection details", Fields: details, Collapsible: true}, {Title: "Using this account", Lines: []string{"Choose this connection in a chat or follow-up agreement. Reconnecting as a different account does not change an existing follow-up's reviewed account.", "Saved credentials describe local state. Check access for a live read test."}, Collapsible: true}}
	digest := connectorLoginReview(d)
	review := []string{id, digest, credentialReview(prior)}
	if status.ID == id && !status.Done {
		screen.Panel.Notice = "Finish sign-in in your browser. This window will update when it completes."
		screen.Panel.Links = []uisnapshot.Link{{Label: "Continue in browser", URL: status.URL}}
		screen.Actions = append(screen.Actions, workapp.ControlAction{Key: "C", Label: "Cancel sign-in", Action: "cancel-signin", Args: []string{id}})
	} else {
		if status.ID == id && status.Done {
			screen.Panel.Notice = status.Issue
			if status.Issue != "" {
				screen.Panel.NoticeTone = "error"
			}
			if screen.Panel.Notice == "" {
				screen.Panel.Notice = status.Notice
				if screen.Panel.Notice == "" {
					screen.Panel.Notice = "Sign-in completed. Check access to verify this account."
				}
			}
		}
		if d.Authentication == connector.AuthOAuth {
			label := "Sign in"
			if prior != nil {
				label = "Reconnect account"
			}
			screen.Actions = append(screen.Actions, workapp.ControlAction{Key: "S", Label: label, Action: "signin", Args: []string{id, digest}, Review: true, Description: "Sign in using your browser for " + d.Name + ". This replaces only this connection's credentials. Requested permissions: " + d.OAuthScopes, Fields: []workapp.ControlField{{Label: "OAuth client secret (optional)", Secret: true, Help: "Only if your own desktop OAuth client requires it. Leave blank to keep a stored secret."}}})
			if usesBundledGoogle(d) {
				a := &screen.Actions[len(screen.Actions)-1]
				a.Fields = nil
				a.Args = append(a.Args, "")
			}
			if d.Kind == connector.KindGoogle {
				if slices.Contains(strings.Fields(d.OAuthScopes), connector.GoogleSendOAuthScope) {
					screen.Actions = append(screen.Actions, workapp.ControlAction{Key: "E", Label: "Turn off email sending", Action: "disable-email-sending", Args: review, Review: true, Description: "Remove sending permission from this connection for new tasks. Active tasks retain their reviewed connection until they finish. Reading stays available. Google's grant remains until you revoke it in Google account settings."})
				} else if prior != nil {
					requested, err := gmailSendingDescriptor(d, true)
					if err != nil {
						return screen, err
					}
					a := workapp.ControlAction{Key: "E", Label: "Enable email sending", Action: "enable-email-sending", Args: review, Review: true, Description: "Sign in again to request sending alongside existing API permissions. Review the account in your browser. Existing access stays configured until authorization succeeds. Choosing this account in chat and approving each exact send are still required. Requested permissions: " + strings.Join(withoutGooglePlannerScopes(strings.Fields(requested.OAuthScopes)), " "), Fields: []workapp.ControlField{{Label: "OAuth client secret (optional)", Secret: true, Help: "Leave blank to keep the stored secret for your own desktop OAuth client."}}}
					if usesBundledGoogle(d) {
						a.Fields = nil
						a.Args = append(append([]string(nil), a.Args...), "")
					}
					screen.Actions = append(screen.Actions, a)
				}
			}
		} else if d.Authentication == connector.AuthBearer {
			screen.Actions = append(screen.Actions, workapp.ControlAction{Key: "S", Label: "Set access token", Action: "token", Args: review, Review: true, Description: "Replace the private token for " + d.Name + " at " + d.Resource + ". No chat or follow-up permissions change.", Fields: []workapp.ControlField{{Label: "Access token", Secret: true, Help: "Enter a new access token. It is stored privately on this device."}}})
		}
		if prior != nil || d.Authentication == connector.AuthNone {
			screen.Actions = append(screen.Actions, workapp.ControlAction{Key: "V", Label: "Check account & read access", Action: "verify", Args: []string{id, digest}})
		}
		if prior != nil {
			screen.Actions = append(screen.Actions, workapp.ControlAction{Key: "D", Label: "Disconnect account", Action: "disconnect", Args: review, Review: true, Description: "Remove this connection's credentials from this device. New reads and follow-ups using it will need sign-in. Requests already sent may finish. Service-side authorization remains until you revoke it at the service."})
		}
		screen.Actions = append(screen.Actions, workapp.ControlAction{Key: "X", Label: "Remove connection", Action: "remove", Args: review, Review: true, Description: "Remove this connection and its local credentials. Chats and follow-ups keep their references and will need their sources reviewed. Service-side authorization is unchanged."})
	}
	screen.Actions = append(screen.Actions, workapp.ControlAction{Key: "R", Label: "Refresh", Action: "status", Args: []string{id}})
	return screen, nil
}

func nativeCredentialStatus(d connector.Descriptor, s auth.Store) (string, error) {
	if d.Authentication == connector.AuthNone {
		return "No sign-in required · access not checked", nil
	}
	c, err := connectionCredential(s, d)
	if err != nil {
		return "", err
	}
	if c == nil {
		return "Not connected", nil
	}
	if c.Expired(time.Now()) {
		if c.Refresh != "" {
			return "Saved sign-in · refresh needed", nil
		}
		return "Sign-in expired", nil
	}
	return "Credentials saved · access not checked", nil
}

// Probe only the granted read surfaces, never write permissions. One successful
// service read does not establish access to every service on the account.
type connectionReadProbe struct {
	operation, label string
	input            json.RawMessage
}

func connectionReadProbes(d connector.Descriptor, now time.Time) []connectionReadProbe {
	if d.Kind != connector.KindGoogle || d.Authentication != connector.AuthOAuth {
		op, input := connectorTestInvocation(d)
		return []connectionReadProbe{{op, "service", input}}
	}
	scopes := strings.Fields(d.OAuthScopes)
	var probes []connectionReadProbe
	if slices.Contains(scopes, "https://www.googleapis.com/auth/gmail.readonly") {
		probes = append(probes, connectionReadProbe{"gmail_list", "email", json.RawMessage(`{"limit":1}`)})
	}
	if slices.Contains(scopes, "https://www.googleapis.com/auth/drive.metadata.readonly") {
		probes = append(probes, connectionReadProbe{"drive_search", "Drive metadata", json.RawMessage(`{"limit":1}`)})
	}
	return probes
}

func connectionServiceLabel(kind string) string {
	if label := map[string]string{connector.KindGoogle: "Google", connector.KindSlack: "Slack", connector.KindNotion: "Notion", connector.KindAtlassian: "Atlassian", connector.KindHTTPJSON: "JSON API", connector.KindHTTPWebhook: "Webhook", connector.KindRemoteMCP: "MCP service"}[kind]; label != "" {
		return label
	}
	return kind
}
func connectionAccessLabel(d connector.Descriptor) string {
	if d.Kind != connector.KindGoogle {
		return "As allowed by the service token"
	}
	var access []string
	scopes := strings.Fields(d.OAuthScopes)
	for _, item := range []struct{ scope, label string }{{"gmail.readonly", "Read email"}, {"calendar.events.readonly", "Calendar access retired; revoke in Google account settings if no longer needed"}, {"drive.metadata.readonly", "Read Drive metadata"}, {"documents", "Read and edit documents"}, {"spreadsheets", "Read and edit spreadsheets"}, {"gmail.send", "Send email"}} {
		if slices.Contains(scopes, "https://www.googleapis.com/auth/"+item.scope) {
			access = append(access, item.label)
		}
	}
	if len(access) == 0 {
		return "Account identity"
	}
	return strings.Join(access, " · ")
}
