package main

import (
	"regexp"

	"github.com/gongahkia/gator/internal/connector"
	"github.com/gongahkia/gator/internal/workapp"
)

// Set only by the distributor at build time. Desktop OAuth client material is
// extractable from the executable; it is never a user token or a web-app secret.
var googleDesktopClientID string
var googleDesktopClientSecret string

func bundledGoogleAvailable() bool {
	return regexp.MustCompile(`^[A-Za-z0-9_-]+\.apps\.googleusercontent\.com$`).MatchString(googleDesktopClientID) &&
		len(googleDesktopClientID) <= 256 && len(googleDesktopClientSecret) <= 256 &&
		regexp.MustCompile(`^[A-Za-z0-9_-]+$`).MatchString(googleDesktopClientSecret)
}

func usesBundledGoogle(d connector.Descriptor) bool {
	return bundledGoogleAvailable() && d.Kind == connector.KindGoogle && d.Authentication == connector.AuthOAuth &&
		d.OAuthClientID == googleDesktopClientID && d.Resource == "https://www.googleapis.com" &&
		d.OAuthAuthorizeURL == connector.GoogleOAuthAuthorizeURL && d.OAuthTokenURL == connector.GoogleOAuthTokenURL &&
		d.OAuthRedirectURL == connector.GoogleOAuthRedirectURL
}

func bundledGoogleAction() workapp.ControlAction {
	return workapp.ControlAction{Key: "G", Label: "Connect Google email & calendar", Action: "add-google-bundled", Review: true,
		Description: "Use this distribution's Google desktop sign-in. Read email and calendar events; sending and document editing are not requested. After saving, sign in in your browser and select this connection for a chat or follow-up.",
		Fields:      []workapp.ControlField{{Label: "Connection ID", Value: "google", Help: "Choose a different name for an additional account."}, {Label: "Display name", Value: "Google"}}}
}
