package spamoor

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	dto "github.com/prometheus/client_model/go"
	"github.com/prometheus/common/expfmt"
	yaml "gopkg.in/yaml.v3"
)

// API is a thin HTTP client for the spamoor-daemon API
type API struct {
	BaseURL string // e.g., http://127.0.0.1:8080
	client  *http.Client
}

func NewAPI(baseURL string) *API {
	return &API{BaseURL: baseURL, client: &http.Client{Timeout: 2 * time.Second}}
}

type createSpammerReq struct {
	Name             string `json:"name"`
	Description      string `json:"description"`
	Scenario         string `json:"scenario"`
	ConfigYAML       string `json:"config"`
	StartImmediately bool   `json:"startImmediately"`
}

// CreateSpammer posts a new spammer with a YAML-serializable config; returns its ID.
// The config parameter is YAML marshalled and sent as the "config" field expected by the daemon.
func (api *API) CreateSpammer(name, scenario string, config any, start bool) (int, error) {
	bz, err := toYAMLString(config)
	if err != nil {
		return 0, fmt.Errorf("yaml marshal config: %w", err)
	}
	reqBody := createSpammerReq{
		Name:             strings.TrimSpace(name),
		Description:      strings.TrimSpace(name),
		Scenario:         strings.TrimSpace(scenario),
		ConfigYAML:       bz,
		StartImmediately: start,
	}
	b, _ := json.Marshal(reqBody)
	url := fmt.Sprintf("%s/api/spammer", api.BaseURL)
	resp, err := api.client.Post(url, "application/json", bytes.NewReader(b))
	if err != nil {
		return 0, err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(resp.Body)
		return 0, fmt.Errorf("create spammer failed: %s", string(body))
	}
	var id int
	dec := json.NewDecoder(resp.Body)
	if err := dec.Decode(&id); err != nil {
		return 0, fmt.Errorf("decode id: %w", err)
	}
	return id, nil
}

// DeleteSpammer deletes an existing spammer by ID.
func (api *API) DeleteSpammer(id int) error {
	url := fmt.Sprintf("%s/api/spammer/%d", api.BaseURL, id)
	req, _ := http.NewRequest(http.MethodDelete, url, http.NoBody)
	resp, err := api.client.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("delete spammer failed: %s", string(body))
	}
	return nil
}

// StartSpammer sends a start request for a given spammer ID.
func (api *API) StartSpammer(id int) error {
	url := fmt.Sprintf("%s/api/spammer/%d/start", api.BaseURL, id)
	req, _ := http.NewRequest(http.MethodPut, url, http.NoBody)
	resp, err := api.client.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("start spammer failed: %s", string(body))
	}
	return nil
}

// GetMetricsRaw fetches the Prometheus /metrics endpoint and returns the raw text.
func (api *API) GetMetricsRaw() (string, error) {
	url := fmt.Sprintf("%s/metrics", api.BaseURL)
	resp, err := api.client.Get(url)
	if err != nil {
		return "", err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("metrics request failed: %s", string(body))
	}
	b, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

// Spammer represents a spammer resource minimally for status checks.
type Spammer struct {
	ID       int    `json:"id"`
	Name     string `json:"name"`
	Scenario string `json:"scenario"`
	Status   int    `json:"status"`
}

// GetSpammer retrieves a spammer by ID.
func (api *API) GetSpammer(id int) (*Spammer, error) {
	url := fmt.Sprintf("%s/api/spammer/%d", api.BaseURL, id)
	resp, err := api.client.Get(url)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("get spammer failed: %s", string(body))
	}
	var s Spammer
	dec := json.NewDecoder(resp.Body)
	if err := dec.Decode(&s); err != nil {
		return nil, err
	}
	return &s, nil
}

// PauseSpammer pauses a running spammer by ID.
func (api *API) PauseSpammer(id int) error {
	url := fmt.Sprintf("%s/api/spammer/%d/pause", api.BaseURL, id)
	req, _ := http.NewRequest(http.MethodPut, url, http.NoBody)
	resp, err := api.client.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("pause spammer failed: %s", string(body))
	}
	return nil
}

// ListSpammers returns all configured spammers.
func (api *API) ListSpammers() ([]Spammer, error) {
	url := fmt.Sprintf("%s/api/spammers", api.BaseURL)
	resp, err := api.client.Get(url)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("list spammers failed: %s", string(body))
	}
	var s []Spammer
	dec := json.NewDecoder(resp.Body)
	if err := dec.Decode(&s); err != nil {
		return nil, err
	}
	return s, nil
}

// Client represents a daemon RPC client entry.
type Client struct {
	Index   int      `json:"index"`
	Name    string   `json:"name"`
	URL     string   `json:"url"`
	Groups  []string `json:"groups"`
	Enabled bool     `json:"enabled"`
	Height  uint64   `json:"height"`
}

// GetClients lists daemon RPC clients.
func (api *API) GetClients() ([]Client, error) {
	url := fmt.Sprintf("%s/api/clients", api.BaseURL)
	resp, err := api.client.Get(url)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("get clients failed: %s", string(body))
	}
	var out []Client
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, err
	}
	return out, nil
}

// UpdateClientGroups sets the groups for a given client index.
func (api *API) UpdateClientGroups(index int, groups []string) error {
	payload := struct {
		Groups []string `json:"groups"`
	}{Groups: groups}
	b, _ := json.Marshal(payload)
	url := fmt.Sprintf("%s/api/client/%d/groups", api.BaseURL, index)
	req, _ := http.NewRequest(http.MethodPut, url, bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	resp, err := api.client.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("update client groups failed: %s", string(body))
	}
	return nil
}

// UpdateClientName sets the display name for a given client index.
func (api *API) UpdateClientName(index int, name string) error {
	payload := struct {
		Name string `json:"name"`
	}{Name: name}
	b, _ := json.Marshal(payload)
	url := fmt.Sprintf("%s/api/client/%d/name", api.BaseURL, index)
	req, _ := http.NewRequest(http.MethodPut, url, bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	resp, err := api.client.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("update client name failed: %s", string(body))
	}
	return nil
}

// Export returns the daemon export blob as raw JSON/YAML string.
func (api *API) Export() (string, error) {
	url := fmt.Sprintf("%s/api/export", api.BaseURL)
	resp, err := api.client.Get(url)
	if err != nil {
		return "", err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("export failed: %s", string(body))
	}
	b, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

// Import posts an import payload; accepts raw string (JSON or YAML) and content-type.
func (api *API) Import(body string, contentType string) error {
	if contentType == "" {
		contentType = "application/json"
	}
	url := fmt.Sprintf("%s/api/import", api.BaseURL)
	resp, err := api.client.Post(url, contentType, strings.NewReader(body))
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		b, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("import failed: %s", string(b))
	}
	return nil
}

// toYAMLString marshals an input into YAML. If input is already a string, it is returned as-is.
func toYAMLString(in any) (string, error) {
	if in == nil {
		return "", nil
	}
	if s, ok := in.(string); ok {
		return s, nil
	}
	b, err := yaml.Marshal(in)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

// GetMetrics fetches and parses Prometheus metrics into MetricFamily structs.
// Uses expfmt.Decoder to avoid validation scheme panics in TextParser.
func (api *API) GetMetrics() (map[string]*dto.MetricFamily, error) {
	raw, err := api.GetMetricsRaw()
	if err != nil {
		return nil, err
	}
	dec := expfmt.NewDecoder(strings.NewReader(raw), expfmt.NewFormat(expfmt.TypeTextPlain))
	out := make(map[string]*dto.MetricFamily)
	for {
		mf := &dto.MetricFamily{}
		if err := dec.Decode(mf); err != nil {
			if err == io.EOF {
				break
			}
			return nil, err
		}
		if mf.GetName() != "" {
			out[mf.GetName()] = mf
		}
	}
	return out, nil
}
