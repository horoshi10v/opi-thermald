package telegram

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

type Client struct {
	botToken string
	chatID   string
	http     *http.Client
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
