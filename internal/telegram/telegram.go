package telegram

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/url"
	"time"
)

type Client struct {
	botToken string
	chatID   string
	http     *http.Client
}

type UpdateResponse struct {
	OK     bool     `json:"ok"`
	Result []Update `json:"result"`
}

type Update struct {
	UpdateID int64   `json:"update_id"`
	Message  Message `json:"message"`
}

type Message struct {
	Text string `json:"text"`
	Chat struct {
		ID int64 `json:"id"`
	} `json:"chat"`
}

func New(botToken, chatID string) *Client {
	return &Client{
		botToken: botToken,
		chatID:   chatID,
		http:     &http.Client{Timeout: 10 * time.Second},
	}
}

func (c *Client) Enabled() bool {
	return c.botToken != "" && c.chatID != ""
}

func (c *Client) Send(message string) error {
	if !c.Enabled() {
		return nil
	}

	payload := map[string]string{
		"chat_id": c.chatID,
		"text":    message,
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	url := fmt.Sprintf("https://api.telegram.org/bot%s/sendMessage", c.botToken)
	resp, err := c.http.Post(url, "application/json", bytes.NewReader(body))
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= http.StatusBadRequest {
		return fmt.Errorf("telegram returned %s", resp.Status)
	}
	return nil
}

func (c *Client) SendPhoto(filename, caption string, data []byte) error {
	if !c.Enabled() {
		return nil
	}

	var body bytes.Buffer
	writer := multipart.NewWriter(&body)

	if err := writer.WriteField("chat_id", c.chatID); err != nil {
		return err
	}
	if caption != "" {
		if err := writer.WriteField("caption", caption); err != nil {
			return err
		}
	}

	part, err := writer.CreateFormFile("photo", filename)
	if err != nil {
		return err
	}
	if _, err := part.Write(data); err != nil {
		return err
	}
	if err := writer.Close(); err != nil {
		return err
	}

	endpoint := fmt.Sprintf("https://api.telegram.org/bot%s/sendPhoto", c.botToken)
	resp, err := c.http.Post(endpoint, writer.FormDataContentType(), &body)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= http.StatusBadRequest {
		return fmt.Errorf("telegram returned %s", resp.Status)
	}
	return nil
}

func (c *Client) GetUpdates(offset int64) ([]Update, error) {
	if !c.Enabled() {
		return nil, nil
	}

	query := url.Values{}
	query.Set("timeout", "1")
	if offset > 0 {
		query.Set("offset", fmt.Sprintf("%d", offset))
	}

	endpoint := fmt.Sprintf("https://api.telegram.org/bot%s/getUpdates?%s", c.botToken, query.Encode())
	resp, err := c.http.Get(endpoint)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= http.StatusBadRequest {
		return nil, fmt.Errorf("telegram returned %s", resp.Status)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var decoded UpdateResponse
	if err := json.Unmarshal(body, &decoded); err != nil {
		return nil, err
	}
	if !decoded.OK {
		return nil, fmt.Errorf("telegram getUpdates returned ok=false")
	}

	return decoded.Result, nil
}
