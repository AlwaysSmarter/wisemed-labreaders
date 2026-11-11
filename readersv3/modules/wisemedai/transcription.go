package wisemedai

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/json"
	"errors"
	"io"
	"math"
	"mime/multipart"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

const transcriptionModel = "gpt-transcribe"

// The iOS recorder sends a canonical mono PCM16 WAV with no metadata chunks.
func dictationAudioSeconds(wav []byte) (float64, bool) {
	if len(wav) <= 44 || len(wav) > 700000 || (len(wav)-44)%2 != 0 ||
		string(wav[:4]) != "RIFF" || string(wav[8:16]) != "WAVEfmt " ||
		string(wav[36:40]) != "data" {
		return 0, false
	}
	u32 := binary.LittleEndian.Uint32
	u16 := binary.LittleEndian.Uint16
	rate := u32(wav[24:28])
	if rate < 8000 || rate > 192000 || u32(wav[4:8]) != uint32(len(wav)-8) ||
		u32(wav[16:20]) != 16 || u16(wav[20:22]) != 1 || u16(wav[22:24]) != 1 ||
		u32(wav[28:32]) != rate*2 || u16(wav[32:34]) != 2 || u16(wav[34:36]) != 16 ||
		u32(wav[40:44]) != uint32(len(wav)-44) {
		return 0, false
	}
	seconds := float64(len(wav)-44) / float64(rate*2)
	return seconds, seconds <= 8
}

func (m *Module) transcribe(ctx context.Context, tx pgx.Tx, p principal, s Session, t Template, audio []byte, seconds float64) (string, error) {
	if m.cfg.Provider != "openai" || m.openAIKey() == "" {
		return "", errors.New("audio_transcription_not_configured")
	}

	var body bytes.Buffer
	form := multipart.NewWriter(&body)
	file, err := form.CreateFormFile("file", "segment.wav")
	if err != nil {
		return "", err
	}
	if _, err = file.Write(audio); err != nil {
		return "", err
	}
	vocabulary := make([]string, 0, len(t.Fields))
	for _, field := range t.Fields {
		vocabulary = append(vocabulary, field.Label)
	}
	prompt := "Dictare medicală în română. Transcrie fidel ce se aude, inclusiv negațiile, cifrele și unitățile. Nu completa cuvinte care nu se aud. Vocabular posibil: " + strings.Join(vocabulary, ", ")
	if len(s.Chunks) > 0 {
		previous := []rune(s.Chunks[len(s.Chunks)-1].Text)
		if len(previous) > 350 {
			previous = previous[len(previous)-350:]
		}
		prompt += "\nContext din segmentul anterior, nu îl repeta: " + string(previous)
	}
	for key, value := range map[string]string{"model": transcriptionModel, "languages": `["ro"]`, "prompt": prompt} {
		if err = form.WriteField(key, value); err != nil {
			return "", err
		}
	}
	if err = form.Close(); err != nil {
		return "", err
	}

	// Reserve separately so a later extraction or database error cannot erase provider usage.
	if _, err = tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtext($1))`, p.Tenant); err != nil {
		return "", err
	}
	var spent float64
	if err = tx.QueryRow(ctx, `SELECT coalesce(sum(cost_usd),0) FROM wai_usage_records WHERE tenant_id=$1 AND created_at>=date_trunc('month',now())`, p.Tenant).Scan(&spent); err != nil {
		return "", err
	}
	// Transcription is duration-priced. Round the reservation up to whole seconds.
	reservedCost := math.Ceil(seconds) * 0.0045 / 60
	quotaUnits := int64(math.Ceil(seconds * 100))
	if spent+reservedCost > m.cfg.MonthlyBudget {
		return "", errors.New("monthly_budget_exceeded")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://api.openai.com/v1/audio/transcriptions", &body)
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", "Bearer "+m.openAIKey())
	req.Header.Set("Content-Type", form.FormDataContentType())
	conn, err := pgx.ConnectConfig(ctx, m.db.Config().ConnConfig.Copy())
	if err != nil {
		return "", errors.New("usage_database_unavailable")
	}
	defer conn.Close(context.Background())
	id := uuid.NewString()
	scope := aiUsageContext{Session: s.ID, Template: s.Version, Operation: "audio_transcription"}
	scope.encryptedPrompt, err = m.box.seal([]promptMessage{{Role: "prompt", Content: prompt}}, promptScope(p.Tenant, id))
	if err != nil {
		return "", err
	}
	if err = reserveTokens(ctx, conn, p, id, transcriptionModel, quotaUnits, reservedCost, scope); err != nil {
		return "", err
	}
	settled := false
	defer func() {
		if !settled {
			cleanup, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			defer cancel()
			_, _ = conn.Exec(cleanup, `UPDATE wai_usage_records SET status='uncertain' WHERE id=$1 AND status='reserved'`, id)
		}
	}()
	start := time.Now()
	resp, err := m.client.Do(req)
	if err != nil {
		return "", errors.New("audio_transcription_failed")
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		if resp.StatusCode >= 400 && resp.StatusCode < 500 {
			_, err = conn.Exec(ctx, `UPDATE wai_usage_records SET status='rejected',cost_usd=0 WHERE id=$1`, id)
			settled = err == nil
		}
		return "", errors.New("audio_transcription_failed")
	}
	var result struct {
		Text  string `json:"text"`
		Usage struct {
			Type   string `json:"type"`
			Input  int    `json:"input_tokens"`
			Output int    `json:"output_tokens"`
		} `json:"usage"`
	}
	if err = json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&result); err != nil {
		return "", errors.New("audio_transcription_failed")
	}
	// Duration-priced replies may omit token usage. Count the reserved quota units in that case.
	input, output := result.Usage.Input, result.Usage.Output
	if input+output == 0 {
		input = int(quotaUnits)
	}
	encryptedOutput, err := m.box.seal(promptOutput{Content: result.Text}, promptScope(p.Tenant, id)+":output")
	if err != nil {
		return "", err
	}
	_, err = conn.Exec(ctx, `UPDATE wai_usage_records SET input_tokens=$2,output_tokens=$3,status='completed',latency_ms=$4,encrypted_output=CASE WHEN encrypted_prompt IS NOT NULL THEN $5::bytea END WHERE id=$1`, id, input, output, time.Since(start).Milliseconds(), encryptedOutput)
	if err != nil {
		return "", errors.New("usage_database_unavailable")
	}
	settled = true
	return strings.TrimSpace(result.Text), nil
}
