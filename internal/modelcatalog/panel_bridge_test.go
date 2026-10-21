package modelcatalog

import (
	"encoding/json"
	"strings"
	"testing"

	modelprovider "github.com/gongahkia/gator/internal/model"
)

func TestDesktopCloudFieldsPreserveSecretRedaction(t *testing.T) {
	p := NewModelCatalogPanel(Config{})
	form := newCloudModelSetupForm(modelprovider.OpenAI, "test-model", "", nil, 80)
	p.model.screen = localModelsScreen
	p.model.localModels.cloudSetup = &form
	panel := p.UISnapshot()
	for i, row := range panel.Rows {
		if row.Input == nil {
			t.Fatal("cloud field has no native input metadata")
		}
		if row.Input.Secret {
			p.SetBridgeField(i, "private-test-key")
			p.SelectBridgeRow(i)
			if form.focus != cloudSetupCredentialField || form.credential.Value() != "private-test-key" {
				t.Fatal("native credential edit did not reach its existing Go field")
			}
		}
	}
	encoded, _ := json.Marshal(p.UISnapshot())
	if strings.Contains(string(encoded), "private-test-key") {
		t.Fatal("desktop projection exposed a credential")
	}
}
