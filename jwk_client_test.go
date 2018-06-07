package auth0

import (
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"gopkg.in/square/go-jose.v2"
)

type mockKeyCacher struct {
	getError error
	addError error
	keyID    string
}

func newMockKeyCacher(getError error, addError error, keyID string) *mockKeyCacher {
	return &mockKeyCacher{
		getError,
		addError,
		keyID,
	}
}

func (mockKC *mockKeyCacher) Get(keyID string) (*jose.JSONWebKey, error) {
	if mockKC.getError == nil {
		mockKey := jose.JSONWebKey{Use: "testGet"}
		mockKey.KeyID = mockKC.keyID
		return &mockKey, nil
	}
	return nil, ErrNoKeyFound
}

func (mockKC *mockKeyCacher) Add(keyID string, webKeys []jose.JSONWebKey) (*jose.JSONWebKey, error) {
	if mockKC.addError == nil {
		mockKey := jose.JSONWebKey{Use: "testAdd"}
		mockKey.KeyID = mockKC.keyID
		return &mockKey, nil
	}
	return nil, ErrNoKeyFound
}

func TestJWKDownloadKeySuccess(t *testing.T) {
	opts, tokenRS256, tokenES384, err := genNewTestServer(true)
	if err != nil {
		t.Error(err)
		t.FailNow()
	}
	client := NewJWKClient(opts, nil)

	keys, err := client.downloadKeys()
	if err != nil || len(keys) < 1 {
		t.Errorf("The keys should have been correctly received: %v", err)
		t.FailNow()
	}

	for _, token := range []string{tokenRS256, tokenES384} {
		req, _ := http.NewRequest("", "http://localhost", nil)
		headerValue := fmt.Sprintf("Bearer %s", token)
		req.Header.Add("Authorization", headerValue)

		_, err = client.GetSecret(req)
		if err != nil {
			t.Errorf("Should be considered as valid, but failed with error: " + err.Error())
		}
	}
}

func TestJWKDownloadKeyNoKeys(t *testing.T) {
	opts, _, tokenES384, err := genNewTestServer(false)
	client := NewJWKClient(opts, nil)

	req, _ := http.NewRequest("", "http://localhost", nil)
	headerValue := fmt.Sprintf("Bearer %s", tokenES384)
	req.Header.Add("Authorization", headerValue)

	_, err = client.GetSecret(req)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "no Keys has been found")
}

func TestJWKDownloadKeyNotFound(t *testing.T) {
	opts, _, _, err := genNewTestServer(true)
	if err != nil {
		t.Error(err)
		t.FailNow()
	}
	client := NewJWKClient(opts, nil)

	invalidToken := getTestTokenWithKid(defaultAudience, defaultIssuer, time.Now().Add(24*time.Hour), jose.ES384, genECDSAJWK(jose.ES384, "keyES385"), "keyES385")

	req, _ := http.NewRequest("", "http://localhost", nil)
	headerValue := fmt.Sprintf("Bearer %s", invalidToken)
	req.Header.Add("Authorization", headerValue)

	_, err = client.GetSecret(req)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "no Keys has been found")
}

func TestJWKDownloadKeyInvalid(t *testing.T) {

	// Invalid content
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintln(w, "Invalid Data")
	}))

	opts := JWKClientOptions{URI: ts.URL}
	client := NewJWKClient(opts, nil)

	_, err := client.downloadKeys()
	if err != ErrInvalidContentType {
		t.Errorf("An ErrInvalidContentType should be returned in case of invalid Content-Type Header.")
	}

	// Invalid Payload
	ts = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintln(w, "Invalid Data")
	}))

	opts = JWKClientOptions{URI: ts.URL}
	client = NewJWKClient(opts, nil)

	_, err = client.downloadKeys()
	if err == nil {
		t.Errorf("An non JSON payload should return an error.")
	}
}

func TestGetKeyOfJWKClient(t *testing.T) {
	opts, _, _, err := genNewTestServer(true)
	if err != nil {
		t.Error(err)
		t.FailNow()
	}

	tests := []struct {
		name             string
		mkc              *mockKeyCacher
		expectedErrorMsg string
	}{
		{
			name: "pass - custom cacher get key",
			mkc: newMockKeyCacher(
				nil,
				nil,
				"key1",
			),
			expectedErrorMsg: "",
		},
		{
			name: "pass - custom cacher add key",
			mkc: newMockKeyCacher(
				errors.New("Key not in cache"),
				nil,
				"key1",
			),
			expectedErrorMsg: "",
		},
		{
			name: "fail - custom cacher add invalid key",
			mkc: newMockKeyCacher(
				errors.New("Key not in cache"),
				ErrNoKeyFound,
				"key1",
			),
			expectedErrorMsg: "no Keys has been found",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			client := NewJWKClientWithCustomCacher(opts, nil, test.mkc)
			_, err := client.GetKey("key1")
			if test.expectedErrorMsg != "" {
				if err == nil {
					t.Errorf("Validation should have failed with error with substring: " + test.expectedErrorMsg)
				} else if !strings.Contains(err.Error(), test.expectedErrorMsg) {
					t.Errorf("Validation should have failed with error with substring: " + test.expectedErrorMsg + ", but got: " + err.Error())
				}
			} else {
				if err != nil {
					t.Errorf("Validation should not have failed with error, but got: " + err.Error())
				}
			}
		})
	}
}

func TestCreateJWKClientCustomCacher(t *testing.T) {
	opts, _, _, err := genNewTestServer(true)
	if err != nil {
		t.Error(err)
		t.FailNow()
	}

	tests := []struct {
		name      string
		keyCacher KeyCacher
	}{
		{
			name:      "pass- no key cacher",
			keyCacher: nil,
		},
		{
			name:      "pass- custome key cacher",
			keyCacher: NewMemoryKeyCacher(time.Duration(100), 5),
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			client := NewJWKClientWithCustomCacher(opts, nil, test.keyCacher)
			assert.NotEmpty(t, client.keyCacher)
		})
	}
}
