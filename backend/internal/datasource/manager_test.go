package datasource

import (
	"testing"

	"data-vision/backend/internal/model"
)

func TestCredentialsAreEncryptedAndRoundTrip(t *testing.T) {
	manager := NewManager("unit-test-key")
	secret, err := manager.Encrypt(Credentials{Password: "password", Token: "token", Headers: map[string]string{"X-API-Key": "api-key"}})
	if err != nil {
		t.Fatal(err)
	}
	if secret == "" || secret == "password" || secret == "token" {
		t.Fatalf("secret was not encrypted: %q", secret)
	}
	credentials, err := manager.Decrypt(model.DataSource{SecretJSON: secret})
	if err != nil {
		t.Fatal(err)
	}
	if credentials.Password != "password" || credentials.Token != "token" || credentials.Headers["X-API-Key"] != "api-key" {
		t.Fatalf("unexpected credentials: %#v", credentials)
	}
}

func TestHTTPBaseURLRejectsEmbeddedCredentials(t *testing.T) {
	_, err := HTTPBaseURL(model.DataSource{Type: TypeHTTP, ConfigJSON: `{"baseUrl":"https://user:password@example.com"}`})
	if err == nil {
		t.Fatal("expected embedded credentials to be rejected")
	}
}
