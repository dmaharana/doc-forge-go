package service

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"net/smtp"
	"time"

	"docforge/internal/config"
	"docforge/internal/crypto"
	"docforge/internal/models"
)

type NotificationService struct {
	cfg *config.Config
}

func NewNotificationService(cfg *config.Config) *NotificationService {
	return &NotificationService{cfg: cfg}
}

func (s *NotificationService) SendGenerationNotification(
	ctx context.Context,
	user *models.User,
	project *models.Project,
	gen *models.Generation,
	docsCount int,
	prefs []models.NotificationPreference,
) {
	subject := fmt.Sprintf("[DocForge] Documentation Generation Complete: %s", project.Name)
	message := fmt.Sprintf(
		"Hello %s,\n\nYour technical documentation generation request for project '%s' has completed successfully.\n\nSummary:\n- Status: %s\n- Model Used: %s\n- Documents Synthesized: %d\n- Finished At: %s\n\nYou can now review the draft documentation, add structured feedback, and finalize approvals in DocForge.\n\nBest regards,\nDocForge Team",
		user.Name, project.Name, gen.Status, gen.ModelUsed, docsCount, time.Now().Format(time.RFC1123),
	)

	// Send to enabled channels
	emailSent := false
	teamsSent := false

	for _, pref := range prefs {
		if !pref.Enabled {
			continue
		}

		switch pref.Channel {
		case models.NotificationChannelEmail:
			if err := s.sendEmail(user.Email, subject, message); err != nil {
				log.Printf("[NOTIFY] Error sending email to %s: %v", user.Email, err)
			} else {
				emailSent = true
			}

		case models.NotificationChannelTeams:
			webhookURL, err := crypto.Decrypt(pref.TeamsWebhookURLEncrypted, s.cfg.EncryptionKey)
			if err != nil || webhookURL == "" {
				log.Printf("[NOTIFY] Failed to decrypt Teams webhook URL: %v", err)
				continue
			}
			if err := s.sendTeamsWebhook(ctx, webhookURL, project.Name, gen, docsCount); err != nil {
				log.Printf("[NOTIFY] Error sending Teams webhook: %v", err)
			} else {
				teamsSent = true
			}
		}
	}

	// Default fallback: If no preferences explicitly set, send email by default
	if !emailSent && !teamsSent && len(prefs) == 0 {
		_ = s.sendEmail(user.Email, subject, message)
	}
}

func (s *NotificationService) sendEmail(to, subject, body string) error {
	if s.cfg.SMTPHost == "" {
		// Log to console for MVP audit
		log.Printf("\n==================== [DOCFORGE EMAIL NOTIFICATION] ====================\nTO: %s\nFROM: %s\nSUBJECT: %s\n\n%s\n=======================================================================\n",
			to, s.cfg.SMTPFrom, subject, body)
		return nil
	}

	msg := []byte(fmt.Sprintf("To: %s\r\nFrom: %s\r\nSubject: %s\r\n\r\n%s\r\n", to, s.cfg.SMTPFrom, subject, body))
	addr := fmt.Sprintf("%s:%d", s.cfg.SMTPHost, s.cfg.SMTPPort)

	var auth smtp.Auth
	if s.cfg.SMTPUser != "" {
		auth = smtp.PlainAuth("", s.cfg.SMTPUser, s.cfg.SMTPPass, s.cfg.SMTPHost)
	}

	return smtp.SendMail(addr, auth, s.cfg.SMTPFrom, []string{to}, msg)
}

func (s *NotificationService) sendTeamsWebhook(ctx context.Context, webhookURL, projectName string, gen *models.Generation, docsCount int) error {
	// Microsoft Teams MessageCard format
	payload := map[string]any{
		"@type":      "MessageCard",
		"@context":   "http://schema.org/extensions",
		"themeColor": "0076D7",
		"summary":    fmt.Sprintf("DocForge Generation Completed: %s", projectName),
		"sections": []map[string]any{
			{
				"activityTitle":    fmt.Sprintf("DocForge: Documentation Generated for **%s**", projectName),
				"activitySubtitle": fmt.Sprintf("Completed at %s", time.Now().Format("2006-01-02 15:04:05 UTC")),
				"facts": []map[string]string{
					{"name": "Status", "value": string(gen.Status)},
					{"name": "Model", "value": gen.ModelUsed},
					{"name": "Documents Created", "value": fmt.Sprintf("%d", docsCount)},
				},
				"markdown": true,
			},
		},
	}

	jsonBytes, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	req, err := http.NewRequestWithContext(ctx, "POST", webhookURL, bytes.NewReader(jsonBytes))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 300 {
		return fmt.Errorf("teams webhook returned status %d", resp.StatusCode)
	}

	log.Printf("[NOTIFY] Microsoft Teams webhook delivered successfully for project %s", projectName)
	return nil
}
