package goodmem

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/PAIR-Systems-Inc/goodmem-go/models"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestDecisionEvaluationPreservesStructuredContent(t *testing.T) {
	precise := `{"amount":12345678901234567890.123456789}`
	instructions := "Use the given facts"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" || r.URL.Path != "/v1/decision-models/model-1/decide" {
			t.Errorf("route: %s %s", r.Method, r.URL)
		}
		var body struct {
			State     models.DecisionContent             `json:"state"`
			Questions map[string]models.DecisionQuestion `json:"questions"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Error(err)
		}
		if body.State.JSONValue == nil || *body.State.JSONValue != precise {
			t.Errorf("precision lost: %#v", body.State)
		}
		if body.Questions["eligible"].Noul == nil {
			t.Error("question missing")
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"model":"fixture","answers":{"eligible":{"noul":{"probabilityTrue":0.75}}},"usage":{"inputTokens":0}}`)
	}))
	defer server.Close()
	client, err := New(server.URL, "fixture-key")
	if err != nil {
		t.Fatal(err)
	}
	response, err := client.DecisionModels().Decide(context.Background(), "model-1", &models.DecisionRequest{
		State:     models.DecisionContent{JSONValue: &precise},
		Questions: map[string]models.DecisionQuestion{"eligible": {Instructions: &models.DecisionContent{Text: &instructions}, Noul: &models.DecisionNoulQuestion{}}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if response.Answers["eligible"].Noul.ProbabilityTrue != 0.75 || response.Usage.InputTokens == nil || *response.Usage.InputTokens != 0 || response.Usage.OutputTokens != nil {
		t.Fatalf("response: %#v", response)
	}
}

func TestDecisionQuestionMapValidatesBranches(t *testing.T) {
	text := "hello"
	request := models.DecisionRequest{State: models.DecisionContent{Text: &text}, Questions: map[string]models.DecisionQuestion{"bad": {}}}
	if err := request.Validate(); err == nil {
		t.Fatal("empty question in typed map should fail validation")
	}
}

func TestDecisionListRetainsContinuation(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("pageToken") != "page-1" || r.URL.Query().Get("providerType") != "LAYA" {
			t.Errorf("query: %s", r.URL)
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"decisionModels":[],"totalCount":8,"nextPageToken":"page-2"}`)
	}))
	defer server.Close()
	client, _ := New(server.URL, "fixture-key")
	token := "page-1"
	provider := models.DecisionModelProviderTypeLAYA
	response, err := client.DecisionModels().List(context.Background(), &DecisionModelsListParams{PageToken: &token, ProviderType: &provider})
	if err != nil {
		t.Fatal(err)
	}
	if response.TotalCount != 8 || response.NextPageToken == nil || *response.NextPageToken != "page-2" {
		t.Fatalf("pagination: %#v", response)
	}
}
