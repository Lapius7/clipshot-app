// Package uploader sends image bytes to a clipshot-server instance.
package uploader

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"strings"
	"time"
)

var ErrInsecureURL = fmt.Errorf("instance url must start with https://")

type Client struct {
	InstanceURL string
	Token       string
	HTTPClient  *http.Client
}

func New(instanceURL, token string) (*Client, error) {
	if !strings.HasPrefix(instanceURL, "https://") {
		return nil, ErrInsecureURL
	}
	return &Client{
		InstanceURL: strings.TrimRight(instanceURL, "/"),
		Token:       token,
		HTTPClient:  &http.Client{Timeout: 30 * time.Second},
	}, nil
}

type uploadResponse struct {
	URL   string `json:"url"`
	Error string `json:"error"`
}

func (c *Client) Upload(filename, contentType string, data []byte) (string, error) {
	var body bytes.Buffer
	w := multipart.NewWriter(&body)

	part, err := w.CreateFormFile("file", filename)
	if err != nil {
		return "", fmt.Errorf("create form file: %w", err)
	}
	if _, err := io.Copy(part, bytes.NewReader(data)); err != nil {
		return "", fmt.Errorf("write form file: %w", err)
	}
	if err := w.Close(); err != nil {
		return "", fmt.Errorf("close multipart writer: %w", err)
	}

	req, err := http.NewRequest(http.MethodPost, c.InstanceURL+"/api/upload", &body)
	if err != nil {
		return "", fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", w.FormDataContentType())
	req.Header.Set("Authorization", "Bearer "+c.Token)

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("upload request failed: %w", err)
	}
	defer resp.Body.Close()

	var out uploadResponse
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return "", fmt.Errorf("server returned unexpected response (status %d)", resp.StatusCode)
	}

	if resp.StatusCode != http.StatusCreated {
		if out.Error != "" {
			return "", fmt.Errorf("upload failed: %s (status %d)", out.Error, resp.StatusCode)
		}
		return "", fmt.Errorf("upload failed with status %d", resp.StatusCode)
	}

	return out.URL, nil
}
