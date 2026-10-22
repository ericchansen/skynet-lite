package main

import (
	"bufio"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"
)

func TestNormalize(t *testing.T) {
	body, repairs, err := normalize([]byte(`{
		"model":"qwen38","stream":true,"reasoning_effort":"high",
		"max_completion_tokens":25000,"unknown":{"keep":true},
		"messages":[
			{"role":"user","content":"hello"},
			{"role":"assistant","content":null,"reasoning":"thinking"},
			{"role":"assistant","tool_calls":[{"id":"a","type":"function","function":{"name":"test","arguments":"{}"}}]},
			{"role":"tool","tool_call_id":"a","content":"OK"}
		]}`))
	if err != nil || repairs != 5 {
		t.Fatalf("normalize: repairs=%d err=%v", repairs, err)
	}

	var request map[string]any
	if err := json.Unmarshal(body, &request); err != nil {
		t.Fatal(err)
	}
	if request["reasoning_effort"] != "medium" || request["max_tokens"] != float64(8192) || request["stream"] != true {
		t.Fatalf("unexpected options: %s", body)
	}
	if _, ok := request["max_completion_tokens"]; ok {
		t.Fatal("output limit was not translated")
	}
	messages := request["messages"].([]any)
	assistant := messages[1].(map[string]any)
	toolCall := messages[2].(map[string]any)
	if assistant["content"] != "" || assistant["reasoning"] != "thinking" || toolCall["content"] != "" ||
		len(toolCall["tool_calls"].([]any)) != 1 || request["unknown"] == nil {
		t.Fatalf("history lost fields: %s", body)
	}
}

func TestStreamFlushAndCancellation(t *testing.T) {
	canceled := make(chan struct{})
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = io.WriteString(w, "data: first\n\n")
		w.(http.Flusher).Flush()
		select {
		case <-r.Context().Done():
			close(canceled)
		case <-time.After(5 * time.Second):
			t.Error("downstream cancellation did not reach Ollama")
		}
	}))
	defer upstream.Close()
	target, _ := url.Parse(upstream.URL)
	server := httptest.NewServer(handler(target))
	defer server.Close()
	client := &http.Client{Timeout: 3 * time.Second}
	response, err := client.Post(server.URL+"/v1/chat/completions", "application/json",
		strings.NewReader(`{"model":"qwen38","messages":[],"stream":true}`))
	if err != nil {
		t.Fatal(err)
	}
	line, err := bufio.NewReader(response.Body).ReadString('\n')
	_ = response.Body.Close()
	if err != nil || line != "data: first\n" {
		t.Fatalf("stream was buffered: %q %v", line, err)
	}
	select {
	case <-canceled:
	case <-time.After(3 * time.Second):
		t.Fatal("upstream request continued after client cancellation")
	}
}

func TestOtherModelAndInvalidInput(t *testing.T) {
	body, _, err := normalize([]byte(`{"model":"other","reasoning_effort":"high","max_tokens":25000,"messages":[{"role":"user","content":"hi"}]}`))
	if err != nil || !strings.Contains(string(body), `"high"`) || !strings.Contains(string(body), `25000`) {
		t.Fatalf("other model settings changed: %s %v", body, err)
	}
	for _, input := range []string{`null`, `{`, `{"model":"qwen38","messages":[],"max_tokens":-1}`} {
		if _, _, err := normalize([]byte(input)); err == nil {
			t.Fatalf("accepted invalid request: %s", input)
		}
	}
	body, _, err = normalize([]byte(`{"model":"qwen38","reasoning":{"effort":"high","other":true},"max_tokens":32,"messages":[]}`))
	if err != nil || !strings.Contains(string(body), `"effort":"medium"`) || !strings.Contains(string(body), `"max_tokens":32`) {
		t.Fatalf("nested reasoning or small limit changed incorrectly: %s %v", body, err)
	}
	for _, effort := range []string{"low", "none"} {
		body, _, err = normalize([]byte(`{"model":"qwen38","reasoning":{"effort":"` + effort + `"},"messages":[]}`))
		if err != nil || !strings.Contains(string(body), `"reasoning_effort":"`+effort+`"`) ||
			!strings.Contains(string(body), `"effort":"`+effort+`"`) {
			t.Fatalf("nested %s reasoning was not preserved: %s %v", effort, body, err)
		}
	}
	body, _, err = normalize([]byte(`{"model":"qwen38","max_tokens":16,"max_completion_tokens":32,"messages":[]}`))
	if err != nil || !strings.Contains(string(body), `"max_tokens":16`) {
		t.Fatalf("smaller output budget lost: %s %v", body, err)
	}
}

func TestNoThinkingForCompaction(t *testing.T) {
	body, _, err := normalize([]byte(`{
		"model":"qwen38:latest","reasoning_effort":"none","reasoning":{"effort":"none"},
		"max_tokens":8192,"messages":[{"role":"user","content":"Summarize this conversation."}]
	}`))
	if err != nil {
		t.Fatal(err)
	}
	var request struct {
		Effort    string `json:"reasoning_effort"`
		MaxTokens int    `json:"max_tokens"`
		Reasoning struct {
			Effort string `json:"effort"`
		} `json:"reasoning"`
	}
	if err := json.Unmarshal(body, &request); err != nil {
		t.Fatal(err)
	}
	if request.Effort != "none" || request.Reasoning.Effort != "none" || request.MaxTokens != qwenOutput {
		t.Fatalf("compaction effort or output budget changed: %s", body)
	}
}

func TestProxyPreservesStreamAndErrors(t *testing.T) {
	stream := "data: {\"choices\":[{\"delta\":{\"content\":\"OK\"},\"finish_reason\":null}]}\n\n" +
		"data: {\"choices\":[{\"delta\":{},\"finish_reason\":\"stop\"}]}\n\ndata: [DONE]\n\n"
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/tags" {
			w.Header().Set("Content-Type", "application/json")
			_, _ = io.WriteString(w, `{"models":[]}`)
			return
		}
		body, _ := io.ReadAll(r.Body)
		if strings.Contains(string(body), `"model":"bad"`) {
			http.Error(w, "model not found", http.StatusNotFound)
			return
		}
		if !strings.Contains(string(body), `"content":""`) {
			t.Error("missing repaired content")
		}
		w.Header().Set("Content-Type", "text/event-stream")
		for _, chunk := range strings.SplitAfter(stream, "\n\n") {
			_, _ = io.WriteString(w, chunk)
			w.(http.Flusher).Flush()
		}
	}))
	defer upstream.Close()
	target, _ := url.Parse(upstream.URL)
	server := httptest.NewServer(handler(target))
	defer server.Close()
	response, err := http.Post(server.URL+"/v1/chat/completions", "application/json",
		strings.NewReader(`{"model":"qwen38","messages":[{"role":"assistant","content":null}],"stream":true}`))
	if err != nil {
		t.Fatal(err)
	}
	actual, _ := io.ReadAll(response.Body)
	_ = response.Body.Close()
	if string(actual) != stream || response.Header.Get("Content-Type") != "text/event-stream" {
		t.Fatalf("stream altered: %q", actual)
	}
	response, err = http.Post(server.URL+"/v1/chat/completions", "application/json",
		strings.NewReader(`{"model":"bad","messages":[]}`))
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusNotFound {
		t.Fatalf("upstream error hidden: %d", response.StatusCode)
	}
	response, err = http.Get(server.URL + "/api/tags")
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		t.Fatal("model discovery failed")
	}
}

func TestNativeChatPassThrough(t *testing.T) {
	body := `{"model":"qwen38","think":"low","options":{"num_ctx":81920,"num_predict":256}}`
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		actual, err := io.ReadAll(r.Body)
		if err != nil {
			t.Error(err)
		}
		if r.Method != http.MethodPost || r.URL.Path != "/api/chat" || string(actual) != body {
			t.Errorf("native request changed: %s %s %s", r.Method, r.URL.Path, actual)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"done":true}`)
	}))
	defer upstream.Close()
	target, _ := url.Parse(upstream.URL)
	server := httptest.NewServer(handler(target))
	defer server.Close()
	response, err := http.Post(server.URL+"/api/chat", "application/json", strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	actual, err := io.ReadAll(response.Body)
	if err != nil || response.StatusCode != http.StatusOK || string(actual) != `{"done":true}` {
		t.Fatalf("native response changed: %d %s %v", response.StatusCode, actual, err)
	}
}
