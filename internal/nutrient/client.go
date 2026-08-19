package nutrient

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"time"
)

const (
	defaultBaseURL = "https://api.nutrient.io"
	extractEndpoint = "extraction/extract"
	parseEndpoint   = "extraction/parse"
	signEndpoint    = "build/sign"
	apiVersion      = "2026-05-25"
)

// Client is a thin wrapper around the Nutrient DWS REST API.
type Client struct {
	httpClient    *http.Client
	baseURL       string
	extractionKey string
	processorKey  string
}

// NewClient creates a new Nutrient API client.
func NewClient(extractionKey, processorKey string) *Client {
	return &Client{
		httpClient:    &http.Client{Timeout: 120 * time.Second},
		baseURL:       defaultBaseURL,
		extractionKey: extractionKey,
		processorKey:  processorKey,
	}
}

// ExtractFields calls POST /extraction/extract with a file and schema.
func (c *Client) ExtractFields(filePath string, schema map[string]any, mode string) (*ExtractFieldsResponse, error) {
	instructions := ExtractFieldsRequest{
		Schema: schema,
		ParseConfig: ParseConfig{
			Mode: mode,
		},
		Options: &ExtractOptions{
			IncludeCitations: boolPtr(true),
		},
	}

	return c.postFile(filePath, extractEndpoint, c.extractionKey, instructions)
}

// ParseDocument calls POST /extraction/parse with a file.
func (c *Client) ParseDocument(filePath string, mode string) (*ParseDocumentResponse, error) {
	instructions := ParseDocumentRequest{
		Mode: mode,
		Output: ParseOutput{
			Format: "spatial",
		},
	}

	return c.postFileParse(filePath, parseEndpoint, c.extractionKey, instructions)
}

// postFile sends a multipart POST request with file + instructions JSON.
func (c *Client) postFile(filePath, endpoint, apiKey string, instructions ExtractFieldsRequest) (*ExtractFieldsResponse, error) {
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)

	// Add file
	file, err := os.Open(filePath)
	if err != nil {
		return nil, fmt.Errorf("open file: %w", err)
	}
	defer file.Close()

	part, err := writer.CreateFormFile("file", filepath.Base(filePath))
	if err != nil {
		return nil, fmt.Errorf("create form file: %w", err)
	}
	if _, err := io.Copy(part, file); err != nil {
		return nil, fmt.Errorf("copy file: %w", err)
	}

	// Add instructions JSON
	instructionsJSON, err := json.Marshal(instructions)
	if err != nil {
		return nil, fmt.Errorf("marshal instructions: %w", err)
	}
	if err := writer.WriteField("instructions", string(instructionsJSON)); err != nil {
		return nil, fmt.Errorf("write instructions field: %w", err)
	}

	if err := writer.Close(); err != nil {
		return nil, fmt.Errorf("close writer: %w", err)
	}

	url := fmt.Sprintf("%s/%s", c.baseURL, endpoint)
	req, err := http.NewRequest("POST", url, &body)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}

	req.Header.Set("Content-Type", writer.FormDataContentType())
	req.Header.Set("Authorization", "Bearer "+apiKey)
	req.Header.Set("x-nutrient-api-version", apiVersion)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("execute request: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("API error (HTTP %d): %s", resp.StatusCode, string(respBody))
	}

	var result ExtractFieldsResponse
	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, fmt.Errorf("unmarshal response: %w", err)
	}

	return &result, nil
}

// postFileParse sends a multipart POST request for parse_document.
func (c *Client) postFileParse(filePath, endpoint, apiKey string, instructions ParseDocumentRequest) (*ParseDocumentResponse, error) {
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)

	file, err := os.Open(filePath)
	if err != nil {
		return nil, fmt.Errorf("open file: %w", err)
	}
	defer file.Close()

	part, err := writer.CreateFormFile("file", filepath.Base(filePath))
	if err != nil {
		return nil, fmt.Errorf("create form file: %w", err)
	}
	if _, err := io.Copy(part, file); err != nil {
		return nil, fmt.Errorf("copy file: %w", err)
	}

	instructionsJSON, err := json.Marshal(instructions)
	if err != nil {
		return nil, fmt.Errorf("marshal instructions: %w", err)
	}
	if err := writer.WriteField("instructions", string(instructionsJSON)); err != nil {
		return nil, fmt.Errorf("write instructions field: %w", err)
	}

	if err := writer.Close(); err != nil {
		return nil, fmt.Errorf("close writer: %w", err)
	}

	url := fmt.Sprintf("%s/%s", c.baseURL, endpoint)
	req, err := http.NewRequest("POST", url, &body)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}

	req.Header.Set("Content-Type", writer.FormDataContentType())
	req.Header.Set("Authorization", "Bearer "+apiKey)
	req.Header.Set("x-nutrient-api-version", apiVersion)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("execute request: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("API error (HTTP %d): %s", resp.StatusCode, string(respBody))
	}

	var result ParseDocumentResponse
	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, fmt.Errorf("unmarshal response: %w", err)
	}

	return &result, nil
}

func boolPtr(b bool) *bool { return &b }

// SignPDF signs a PDF using Nutrient's digital signing API.
func (c *Client) SignPDF(filePath string, signatureConfig map[string]any) ([]byte, error) {
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)

	file, err := os.Open(filePath)
	if err != nil {
		return nil, fmt.Errorf("open file: %w", err)
	}
	defer file.Close()

	part, err := writer.CreateFormFile("file", filepath.Base(filePath))
	if err != nil {
		return nil, fmt.Errorf("create form file: %w", err)
	}
	if _, err := io.Copy(part, file); err != nil {
		return nil, fmt.Errorf("copy file: %w", err)
	}

	instructions := map[string]any{
		"parts": []map[string]any{
			{"file": "file"},
		},
		"output": map[string]any{
			"type": "pdf",
		},
		"actions": []map[string]any{
			{
				"type":      "digitallySign",
				"signature": signatureConfig,
			},
		},
	}

	instructionsJSON, _ := json.Marshal(instructions)
	if err := writer.WriteField("instructions", string(instructionsJSON)); err != nil {
		return nil, fmt.Errorf("write instructions: %w", err)
	}

	if err := writer.Close(); err != nil {
		return nil, fmt.Errorf("close writer: %w", err)
	}

	url := fmt.Sprintf("%s/%s", c.baseURL, signEndpoint)
	req, err := http.NewRequest("POST", url, &body)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}

	req.Header.Set("Content-Type", writer.FormDataContentType())
	req.Header.Set("Authorization", "Bearer "+c.processorKey)
	req.Header.Set("x-nutrient-api-version", apiVersion)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("execute request: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("sign API error (HTTP %d): %s", resp.StatusCode, string(respBody))
	}

	return respBody, nil
}
