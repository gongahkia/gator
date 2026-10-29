package main

import (
	"context"
	"strings"
	"testing"

	"github.com/gongahkia/gator/internal/connector"
)

func TestBundledGoogleSetupPreservesScopeAndManualOption(t *testing.T) {
	oldID, oldSecret := googleDesktopClientID, googleDesktopClientSecret
	t.Cleanup(func() { googleDesktopClientID, googleDesktopClientSecret = oldID, oldSecret })
	t.Setenv("GATOR_CONFIG_DIR", t.TempDir())
	root := t.TempDir()
	t.Setenv("GATOR_STATE_DIR", root)
	c := &connectionControls{stateDir: root}
	googleDesktopClientID, googleDesktopClientSecret = "", ""
	if _, err := c.call(context.Background(), "add-google-bundled", []string{"google", "Google"}); err == nil {
		t.Fatal("unconfigured build accepted distribution sign-in")
	}
	googleDesktopClientID, googleDesktopClientSecret = "123-fixture.apps.googleusercontent.com", "fixture-only"
	screen, err := c.screen("")
	if err != nil {
		t.Fatal(err)
	}
	manual, bundled := false, false
	for _, a := range screen.Actions {
		manual = manual || a.Action == "add-google"
		bundled = bundled || a.Action == "add-google-bundled" && len(a.Fields) == 2 && a.Review
	}
	if !manual || !bundled {
		t.Fatal("missing setup option")
	}
	screen, err = c.call(context.Background(), "add-google-bundled", []string{"google", "Personal Google"})
	if err != nil {
		t.Fatal(err)
	}
	d, credentials, err := configuredConnector("google")
	if err != nil {
		t.Fatal(err)
	}
	if !usesBundledGoogle(d) || strings.Contains(d.OAuthScopes, "gmail.send") || strings.Contains(d.OAuthScopes, "/auth/documents") {
		t.Fatal("incorrect client or scope")
	}
	if _, found, err := credentials.Read(d.CredentialRef()); err != nil || found {
		t.Fatal("setup created credentials", err)
	}
	found := false
	for _, a := range screen.Actions {
		if a.Action == "signin" {
			found = true
			if len(a.Fields) != 0 || len(a.Args) != 3 || !a.Review {
				t.Fatal("bundled sign-in still requests client material")
			}
		}
	}
	if !found {
		t.Fatal("missing sign-in")
	}
	for _, mutate := range []func(*connector.Descriptor){
		func(d *connector.Descriptor) { d.OAuthTokenURL = "https://example.com/token" },
		func(d *connector.Descriptor) { d.OAuthClientID = "another-client" },
		func(d *connector.Descriptor) { d.OAuthAuthorizeURL = "https://example.com/authorize" },
		func(d *connector.Descriptor) { d.Resource = "https://example.com" },
		func(d *connector.Descriptor) { d.OAuthRedirectURL = "http://localhost/elsewhere" },
	} {
		changed := d
		mutate(&changed)
		if usesBundledGoogle(changed) {
			t.Fatal("distribution client material could reach a changed endpoint or client")
		}
	}
}
