package web

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestHandlerServesIndexWithoutRedirect(t *testing.T) {
	response := httptest.NewRecorder()
	Handler().ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/", nil))
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d", response.Code)
	}
	if !strings.Contains(response.Body.String(), "Norbot") {
		t.Fatal("index is missing Norbot title")
	}
}
