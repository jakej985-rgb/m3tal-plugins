package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"m3tal-plugin-ai/internal/assistant"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "m3tal-ai:", err)
		os.Exit(1)
	}
}

func run(arguments []string) error {
	flags := flag.NewFlagSet("m3tal-ai", flag.ContinueOnError)
	docsFlag := flags.String("docs", "", "additional documentation file or directory (path-list separated)")
	apiURL := flags.String("api", os.Getenv("M3TAL_API_URL"), "M3TAL Core API URL (default http://localhost:5050)")
	apiToken := flags.String("token", defaultAPIToken(), "M3TAL Core API token (default M3TAL_API_TOKEN or /etc/m3tal/.env)")
	llmURL := flags.String("llm", os.Getenv("M3TAL_LLM_URL"), "M3TAL LLM server URL (default http://127.0.0.1:8080 if available)")
	interactive := flags.Bool("interactive", false, "run an interactive assistant session")
	jsonOutput := flags.Bool("json", false, "print Core payloads as JSON")
	showVersion := flags.Bool("version", false, "print version and exit")
	runEval := flags.Bool("eval", false, "run retrieval and RAG evaluation on built-in question set")
	if err := flags.Parse(arguments); err != nil {
		return err
	}
	if *showVersion {
		fmt.Printf("m3tal-ai %s\n", assistant.Version)
		return nil
	}
	query := strings.TrimSpace(strings.Join(flags.Args(), " "))
	if query == "" && !*interactive && !*runEval {
		return fmt.Errorf("usage: go run ./cmd/m3tal-ai [--docs PATH] [--api URL] [--interactive] [--json] [--version] [--eval] \"question or M3TAL request\"")
	}

	docPaths := defaultDocsPaths()
	if environment := os.Getenv("M3TAL_DOCS_PATHS"); environment != "" {
		docPaths = filepath.SplitList(environment)
	}
	if *docsFlag != "" {
		docPaths = append(docPaths, filepath.SplitList(*docsFlag)...)
	}

	core := assistant.NewCoreAPI(*apiURL, *apiToken)
	agent, err := assistant.New(docPaths, core)
	if err != nil {
		return err
	}
	llmEndpoint := strings.TrimSpace(*llmURL)
	if llmEndpoint == "" {
		llmEndpoint = "http://127.0.0.1:8080"
	}
	if gen := newHTTPGenerator(llmEndpoint); gen != nil {
		agent.SetLanguageModel(gen)
	}
	agent.SetJSONOutput(*jsonOutput)
	stats := agent.Knowledge().Stats()
	fmt.Fprintln(os.Stderr, agent.Knowledge().Report())
	if len(stats.MissingRoots) > 0 {
		fmt.Fprintf(os.Stderr, "warning: %d documentation root(s) not found\n", len(stats.MissingRoots))
	}
	if stats.Files == 0 && stats.Documents == 0 {
		fmt.Fprintln(os.Stderr, "warning: knowledge base is empty; add docs with --docs or set M3TAL_DOCS_PATHS")
	}
	if *runEval {
		evalMetrics := agent.RunEval()
		if *jsonOutput {
			payload, err := json.MarshalIndent(evalMetrics, "", "  ")
			if err != nil {
				return err
			}
			fmt.Println(string(payload))
			if agent.HasLanguageModel() {
				planMetrics := agent.RunPlanEval()
				planPayload, err := json.MarshalIndent(planMetrics, "", "  ")
				if err != nil {
					return err
				}
				fmt.Println(string(planPayload))
			}
			return nil
		}
		fmt.Println(assistant.PrintEvalReport(evalMetrics))
		if agent.HasLanguageModel() {
			fmt.Println(assistant.PrintPlanEvalReport(agent.RunPlanEval()))
		}
		return nil
	}
	if err := agent.PingCore(context.Background()); err != nil {
		fmt.Fprintf(os.Stderr, "warning: %v\n", err)
	}
	reader := bufio.NewReader(os.Stdin)
	confirm := func(action string) bool {
		fmt.Fprintf(os.Stderr, "Type exactly to confirm: %s\n> ", action)
		answer, _ := reader.ReadString('\n')
		return confirmationMatches(action, answer)
	}
	if !*interactive {
		return agent.Ask(context.Background(), query, confirm, os.Stdout)
	}
	for {
		fmt.Fprint(os.Stdout, "m3tal> ")
		line, readErr := reader.ReadString('\n')
		line = strings.TrimSpace(line)
		if readErr != nil && line == "" {
			return nil
		}
		if line == "exit" || line == "quit" {
			return nil
		}
		if line == "" {
			continue
		}
		if err := agent.Ask(context.Background(), line, confirm, os.Stdout); err != nil {
			fmt.Fprintln(os.Stderr, "m3tal-ai:", err)
		}
		if readErr != nil {
			return nil
		}
	}
}

func confirmationMatches(expected, actual string) bool {
	return strings.TrimSpace(actual) == expected
}

func defaultDocsPaths() []string {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil
	}
	appsRoot := filepath.Join(home, "apps")
	paths := []string{
		filepath.Join(appsRoot, "M3tal-core"),
		filepath.Join(appsRoot, "M3tal-Hub"),
		filepath.Join(appsRoot, "m3tal-plugins"),
	}
	manifest := filepath.Join(appsRoot, "M3tal-Hub", "apps", "manifest.yml")
	contents, err := os.ReadFile(manifest)
	if err != nil {
		return paths
	}
	folders := make(map[string]string)
	entries, _ := os.ReadDir(appsRoot)
	for _, entry := range entries {
		if entry.IsDir() {
			folders[strings.ToLower(entry.Name())] = filepath.Join(appsRoot, entry.Name())
		}
	}
	for _, line := range strings.Split(string(contents), "\n") {
		trimmed := strings.TrimSpace(line)
		if !strings.HasPrefix(trimmed, "repo:") {
			continue
		}
		repository := strings.Trim(strings.TrimSpace(strings.TrimPrefix(trimmed, "repo:")), "\"'")
		if path, found := folders[strings.ToLower(repository)]; found {
			paths = append(paths, path)
		}
	}
	return paths
}

func defaultAPIToken() string {
	if token := os.Getenv("M3TAL_API_TOKEN"); token != "" {
		return token
	}
	if data, err := os.ReadFile("/etc/m3tal/.env"); err == nil {
		for _, line := range strings.Split(string(data), "\n") {
			line = strings.TrimSpace(line)
			if strings.HasPrefix(line, "API_TOKEN=") {
				return strings.Trim(strings.TrimPrefix(line, "API_TOKEN="), "\"'")
			}
		}
	}
	return ""
}

type httpGenerator struct {
	client  *http.Client
	baseURL string
}

func newHTTPGenerator(endpoint string) *httpGenerator {
	if endpoint == "" {
		return nil
	}
	endpoint = strings.TrimRight(endpoint, "/")
	client := &http.Client{Timeout: 90 * time.Second}
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, "GET", endpoint+"/healthz", nil)
	if err != nil {
		return nil
	}
	resp, err := client.Do(req)
	if err != nil || resp.StatusCode != http.StatusOK {
		return nil
	}
	_ = resp.Body.Close()
	return &httpGenerator{client: client, baseURL: endpoint}
}

func (h *httpGenerator) Generate(prompt string, maxTokens int, temperature float64, seed int64) (string, error) {
	reqBody := map[string]any{
		"prompt":      prompt,
		"max_tokens":  maxTokens,
		"temperature": temperature,
		"seed":        seed,
	}
	data, err := json.Marshal(reqBody)
	if err != nil {
		return "", err
	}
	req, err := http.NewRequest("POST", h.baseURL+"/v1/generate", bytes.NewReader(data))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := h.client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("llm server returned %s", resp.Status)
	}
	var res struct {
		Text string `json:"text"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&res); err != nil {
		return "", err
	}
	return res.Text, nil
}
