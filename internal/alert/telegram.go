package alert

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

type telegramPayload struct {
	ChatID    string `json:"chat_id"`
	Text      string `json:"text"`
	ParseMode string `json:"parse_mode"`
}

func sendTelegram(botToken, chatID, message string) error {
	payload := telegramPayload{
		ChatID:    chatID,
		Text:      message,
		ParseMode: "Markdown",
	}

	data, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("telegram: marshal: %w", err)
	}

	url := fmt.Sprintf("https://api.telegram.org/bot%s/sendMessage", botToken)

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Post(url, "application/json", bytes.NewReader(data))
	if err != nil {
		return fmt.Errorf("telegram: post: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("telegram: HTTP %d", resp.StatusCode)
	}
	return nil
}
