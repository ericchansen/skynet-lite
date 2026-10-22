package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"time"
)

const maxBody = 32 << 20
const qwenOutput = 8192

func isQwen(model string) bool {
	return model == "qwen38" || model == "qwen38:latest" ||
		model == "hf.co/huihui-ai/Huihui-Qwen3.8-27B-abliterated-GGUF:Huihui-Qwen3.8-27B-abliterated-Q6_K.gguf"
}

// Preserve unknown fields and tool calls; only repair empty assistant content.
func normalize(body []byte) ([]byte, int, error) {
	var request map[string]json.RawMessage
	if err := json.Unmarshal(body, &request); err != nil {
		return nil, 0, err
	}
	if request == nil {
		return nil, 0, fmt.Errorf("request must be a JSON object")
	}
	var messages []map[string]json.RawMessage
	if err := json.Unmarshal(request["messages"], &messages); err != nil {
		return nil, 0, fmt.Errorf("invalid messages: %w", err)
	}
	repairs := 0
	for _, message := range messages {
		var role string
		_ = json.Unmarshal(message["role"], &role)
		content := bytes.TrimSpace(message["content"])
		if role == "assistant" && (len(content) == 0 || bytes.Equal(content, []byte("null"))) {
			message["content"] = json.RawMessage(`""`)
			repairs++
		}
	}
	var model string
	_ = json.Unmarshal(request["model"], &model)
	if isQwen(model) {
		// Preserve explicit thinking controls; normalize unsupported "high" to medium.
		effort := json.RawMessage(`"medium"`)
		if requested := string(request["reasoning_effort"]); requested == `"low"` || requested == `"none"` {
			effort = request["reasoning_effort"]
		}
		if string(request["reasoning_effort"]) != string(effort) {
			request["reasoning_effort"] = effort
			repairs++
		}
		if raw, ok := request["reasoning"]; ok {
			var reasoning map[string]json.RawMessage
			if err := json.Unmarshal(raw, &reasoning); err != nil {
				return nil, 0, fmt.Errorf("invalid reasoning: %w", err)
			}
			if reasoning != nil {
				reasoning["effort"] = effort
				encoded, err := json.Marshal(reasoning)
				if err != nil {
					return nil, 0, err
				}
				request["reasoning"] = encoded
				repairs++
			}
		}
		outputLimit := qwenOutput
		for _, key := range []string{"max_tokens", "max_completion_tokens"} {
			if raw, ok := request[key]; ok {
				var limit int
				if err := json.Unmarshal(raw, &limit); err != nil || limit <= 0 {
					return nil, 0, fmt.Errorf("%s must be a positive integer", key)
				}
				if limit > qwenOutput {
					repairs++
				}
				if limit < outputLimit {
					outputLimit = limit
				}
			}
		}
		if _, ok := request["max_tokens"]; !ok {
			repairs++
		}
		// Ollama's Chat Completions adapter uses max_tokens.
		request["max_tokens"] = json.RawMessage(fmt.Sprint(outputLimit))
		delete(request, "max_completion_tokens")
	}
	encoded, err := json.Marshal(messages)
	if err != nil {
		return nil, 0, err
	}
	request["messages"] = encoded
	encoded, err = json.Marshal(request)
	return encoded, repairs, err
}

func handler(upstream *url.URL) http.Handler {
	proxy := httputil.NewSingleHostReverseProxy(upstream)
	proxy.FlushInterval = -1
	proxy.ErrorHandler = func(w http.ResponseWriter, r *http.Request, err error) {
		log.Printf("upstream error path=%s: %v", r.URL.Path, err)
		http.Error(w, "Ollama upstream request failed", http.StatusBadGateway)
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && r.URL.Path == "/v1/chat/completions" {
			r.Body = http.MaxBytesReader(w, r.Body, maxBody)
			body, err := io.ReadAll(r.Body)
			_ = r.Body.Close()
			if err != nil {
				log.Printf("request body error: %v", err)
				http.Error(w, "unable to read chat request (maximum 32 MiB)", http.StatusBadRequest)
				return
			}
			body, repairs, err := normalize(body)
			if err != nil {
				log.Printf("invalid chat request: %v", err)
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			log.Printf("chat request bytes=%d compatibility_changes=%d", len(body), repairs)
			r.Body = io.NopCloser(bytes.NewReader(body))
			r.ContentLength = int64(len(body))
			r.Header.Del("Content-Length")
		}
		proxy.ServeHTTP(w, r)
	})
}

func main() {
	listen := flag.String("listen", ":11434", "HTTP listen address")
	upstreamAddress := flag.String("upstream", os.Getenv("OLLAMA_UPSTREAM"), "Ollama HTTP URL")
	logFile := flag.String("log-file", "", "Append logs to this file instead of stderr")
	flag.Parse()

	if *logFile != "" {
		file, err := os.OpenFile(*logFile, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
		if err != nil {
			log.Fatal(err)
		}
		defer file.Close()
		log.SetOutput(file)
	}

	upstream, err := url.Parse(*upstreamAddress)
	if err != nil || upstream == nil || upstream.Scheme != "http" || upstream.Host == "" {
		log.Fatal("-upstream or OLLAMA_UPSTREAM must be an http URL")
	}
	server := &http.Server{
		Addr:              *listen,
		Handler:           handler(upstream),
		ReadHeaderTimeout: 10 * time.Second,
	}
	log.Printf("Ollama compatibility endpoint listening on %s", server.Addr)
	log.Fatal(server.ListenAndServe())
}
