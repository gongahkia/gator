package reviewweb

import "testing"

func TestValidateListenAddressRejectsNonLoopback(t *testing.T) {
	if err := ValidateListenAddress("127.0.0.1:0"); err != nil {
		t.Fatalf("loopback listen rejected: %v", err)
	}
	if err := ValidateListenAddress("[::1]:0"); err != nil {
		t.Fatalf("IPv6 loopback listen rejected: %v", err)
	}
	for _, address := range []string{"0.0.0.0:8080", "192.168.1.10:8080", "example.com:8080", "127.0.0.1"} {
		if err := ValidateListenAddress(address); err == nil {
			t.Fatalf("accepted non-loopback listen %q", address)
		}
	}
}

func TestOpenInBrowserRejectsNonLoopbackHTTP(t *testing.T) {
	if err := OpenInBrowser("https://127.0.0.1/"); err == nil {
		t.Fatal("https review URL was accepted")
	}
	if err := OpenInBrowser("http://example.com/"); err == nil {
		t.Fatal("remote http URL was accepted")
	}
	if err := OpenInBrowser("http://user:pass@127.0.0.1/"); err == nil {
		t.Fatal("credentialed URL was accepted")
	}
}
