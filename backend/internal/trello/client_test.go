package trello

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

func newTestClient(t *testing.T, handler http.HandlerFunc) *Client {
	t.Helper()
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	return &Client{
		apiKey:     "test-key",
		token:      "test-token",
		httpClient: server.Client(),
		baseURL:    server.URL,
	}
}

func TestCreateCard_Success(t *testing.T) {
	var gotMethod, gotPath, gotContentType string
	var gotForm url.Values

	client := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		gotPath = r.URL.Path
		gotContentType = r.Header.Get("Content-Type")
		body, _ := io.ReadAll(r.Body)
		gotForm, _ = url.ParseQuery(string(body))

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"id":"card123","url":"https://trello.com/c/abc123/1-bug","shortUrl":"https://trello.com/c/abc123"}`))
	})

	card, err := client.CreateCard(context.Background(), CreateCardInput{
		ListID:      "list1",
		Name:        "[bug] Something broke",
		Description: "steps here",
		LabelIDs:    []string{"label1", "label2"},
	})
	if err != nil {
		t.Fatalf("CreateCard: %v", err)
	}

	if gotMethod != http.MethodPost {
		t.Errorf("method: want POST, got %s", gotMethod)
	}
	if gotPath != "/cards" {
		t.Errorf("path: want /cards, got %s", gotPath)
	}
	if gotContentType != "application/x-www-form-urlencoded" {
		t.Errorf("content-type: got %q", gotContentType)
	}
	if gotForm.Get("key") != "test-key" || gotForm.Get("token") != "test-token" {
		t.Errorf("credentials not sent correctly: %v", gotForm)
	}
	if gotForm.Get("idList") != "list1" {
		t.Errorf("idList: got %q", gotForm.Get("idList"))
	}
	if gotForm.Get("name") != "[bug] Something broke" {
		t.Errorf("name: got %q", gotForm.Get("name"))
	}
	if gotForm.Get("desc") != "steps here" {
		t.Errorf("desc: got %q", gotForm.Get("desc"))
	}
	if gotForm.Get("idLabels") != "label1,label2" {
		t.Errorf("idLabels: got %q", gotForm.Get("idLabels"))
	}

	if card.ID != "card123" {
		t.Errorf("ID: got %q", card.ID)
	}
	if card.URL != "https://trello.com/c/abc123/1-bug" {
		t.Errorf("URL: got %q", card.URL)
	}
	if card.ShortURL != "https://trello.com/c/abc123" {
		t.Errorf("ShortURL: got %q", card.ShortURL)
	}
}

func TestCreateCard_NoLabels(t *testing.T) {
	var gotForm url.Values
	client := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		gotForm, _ = url.ParseQuery(string(body))
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"id":"c1","url":"https://trello.com/c/c1","shortUrl":"https://trello.com/c/c1"}`))
	})

	_, err := client.CreateCard(context.Background(), CreateCardInput{ListID: "list1", Name: "no labels"})
	if err != nil {
		t.Fatalf("CreateCard: %v", err)
	}
	if _, present := gotForm["idLabels"]; present {
		t.Errorf("expected no idLabels param when LabelIDs is empty, got %v", gotForm.Get("idLabels"))
	}
}

func TestCreateCard_NonSuccessStatus(t *testing.T) {
	client := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"message":"invalid value for idList"}`))
	})

	_, err := client.CreateCard(context.Background(), CreateCardInput{ListID: "bad-list", Name: "x"})
	if err == nil {
		t.Fatal("expected error for non-2xx response")
	}
	if !strings.Contains(err.Error(), "400") {
		t.Errorf("expected error to mention status code 400, got: %v", err)
	}
	if !strings.Contains(err.Error(), "invalid value for idList") {
		t.Errorf("expected error to include response body, got: %v", err)
	}
}

func TestCreateCard_NonSuccessStatus_DoesNotLeakCredentials(t *testing.T) {
	client := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"message":"invalid key"}`))
	})

	_, err := client.CreateCard(context.Background(), CreateCardInput{ListID: "list1", Name: "x"})
	if err == nil {
		t.Fatal("expected error for 401 response")
	}
	if strings.Contains(err.Error(), "test-key") || strings.Contains(err.Error(), "test-token") {
		t.Errorf("error message must not contain the API key or token, got: %v", err)
	}
}

func TestCreateCard_MissingListID(t *testing.T) {
	client := &Client{apiKey: "k", token: "t", httpClient: http.DefaultClient, baseURL: defaultBaseURL}
	_, err := client.CreateCard(context.Background(), CreateCardInput{Name: "x"})
	if err == nil {
		t.Fatal("expected error for missing ListID")
	}
}

func TestCreateCard_MissingName(t *testing.T) {
	client := &Client{apiKey: "k", token: "t", httpClient: http.DefaultClient, baseURL: defaultBaseURL}
	_, err := client.CreateCard(context.Background(), CreateCardInput{ListID: "list1"})
	if err == nil {
		t.Fatal("expected error for missing Name")
	}
}

func TestCreateCard_MalformedResponseBody(t *testing.T) {
	client := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`not-json`))
	})

	_, err := client.CreateCard(context.Background(), CreateCardInput{ListID: "list1", Name: "x"})
	if err == nil {
		t.Fatal("expected error for malformed response body")
	}
}

func TestNewClient_DefaultsToRealBaseURL(t *testing.T) {
	c := NewClient("k", "t")
	if c.baseURL != defaultBaseURL {
		t.Errorf("expected default baseURL %q, got %q", defaultBaseURL, c.baseURL)
	}
}
