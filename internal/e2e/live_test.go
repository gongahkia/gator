//go:build livee2e

package e2e

import (
	"os"
	"testing"
)

func TestLivePrerequisites(t *testing.T) {
	if os.Getenv("NORBOT_E2E_LIVE") != "1" {
		t.Skip("set NORBOT_E2E_LIVE=1")
	}
	for _, key := range []string{"NORBOT_E2E_PUBLIC_URL", "NORBOT_E2E_NAMESPACE", "KUBECONFIG", "OPENAI_API_KEY", "TELEGRAM_BOT_TOKEN", "SLACK_BOT_TOKEN", "DISCORD_BOT_TOKEN", "WHATSAPP_ACCESS_TOKEN"} {
		if os.Getenv(key) == "" {
			t.Fatalf("%s is required", key)
		}
	}
}
