// mautrix-simplex - A Matrix-SimpleX puppeting bridge.
// Copyright (C) 2024 Tricked
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU Affero General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// This program is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
// GNU Affero General Public License for more details.
//
// You should have received a copy of the GNU Affero General Public License
// along with this program.  If not, see <https://www.gnu.org/licenses/>.

package connector

import (
	"context"
	"encoding/base64"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/rs/zerolog"
	"maunium.net/go/mautrix/bridgev2"
	"maunium.net/go/mautrix/bridgev2/database"
	"maunium.net/go/mautrix/bridgev2/networkid"
	"maunium.net/go/mautrix/event"

	"go.mau.fi/mautrix-simplex/pkg/simplexclient"
	"go.mau.fi/mautrix-simplex/pkg/simplexid"
)

// simplexSupportedEmojis maps emoji (with and without variant selectors) to the
// single-character emoji that SimpleX accepts. SimpleX only supports these 8 emojis:
// 👍👎😀😂😢❤🚀✅
var simplexSupportedEmojis = map[string]string{
	"👍":  "👍",
	"👍️": "👍",
	"👎":  "👎",
	"👎️": "👎",
	"😀":  "😀",
	"😂":  "😂",
	"😢":  "😢",
	"❤":   "❤",
	"❤️":  "❤",
	"🚀":  "🚀",
	"✅":  "✅",
	"✅️": "✅",
}

// normalizeEmojiForSimplex converts a Matrix emoji to a SimpleX-compatible one.
// Returns the emoji and true if supported, or empty and false if not.
func normalizeEmojiForSimplex(emoji string) (string, bool) {
	if mapped, ok := simplexSupportedEmojis[emoji]; ok {
		return mapped, true
	}
	return "", false
}

var (
	_ bridgev2.EditHandlingNetworkAPI      = (*SimplexClient)(nil)
	_ bridgev2.ReactionHandlingNetworkAPI  = (*SimplexClient)(nil)
	_ bridgev2.RedactionHandlingNetworkAPI = (*SimplexClient)(nil)
)

// HandleMatrixMessage sends a Matrix message to SimpleX.
func (s *SimplexClient) HandleMatrixMessage(ctx context.Context, msg *bridgev2.MatrixMessage) (*bridgev2.MatrixMessageResponse, error) {
	if s.Client == nil {
		return nil, bridgev2.ErrNotLoggedIn
	}
	chatType, chatID, err := simplexid.ParsePortalID(msg.Portal.ID)
	if err != nil {
		return nil, fmt.Errorf("failed to parse portal ID: %w", err)
	}

	content := MatrixToSimplexMsgContent(msg.Content)
	composed := simplexclient.ComposedMessage{
		MsgContent: content,
		Mentions:   map[string]int64{},
	}
	if msg.ReplyTo != nil {
		itemID, err := simplexid.ParseMessageID(msg.ReplyTo.ID)
		if err == nil {
			composed.QuotedItemID = &itemID
		}
	}

	// Handle file/image/video/audio by downloading from Matrix and sending as a file.
	var tmpPathToClean string
	switch msg.Content.MsgType {
	case event.MsgImage, event.MsgVideo, event.MsgAudio, event.MsgFile:
		data, err := s.Main.Bridge.Bot.DownloadMedia(ctx, msg.Content.URL, msg.Content.File)
		if err != nil {
			return nil, fmt.Errorf("%w: %w", bridgev2.ErrMediaDownloadFailed, err)
		}
		tmpDir := filepath.Join(s.Main.Config.FilesFolder, "tmp")
		fileName := msg.Content.Body
		if fileName == "" {
			fileName = "file"
		}
		tmpFile, err := os.CreateTemp(tmpDir, "simplex-send-*-"+filepath.Base(fileName))
		if err != nil {
			return nil, fmt.Errorf("failed to create temp file: %w", err)
		}
		tmpPathToClean = tmpFile.Name()
		if _, err = tmpFile.Write(data); err != nil {
			tmpFile.Close()
			os.Remove(tmpPathToClean)
			return nil, fmt.Errorf("failed to write temp file: %w", err)
		}
		tmpFile.Close()

		mimeType := msg.Content.GetInfo().MimeType
		if mimeType == "" {
			mimeType = http.DetectContentType(data)
		}
		msgType := "file"
		if isImageMime(mimeType) {
			msgType = "image"
		} else if isVideoMime(mimeType) {
			msgType = "video"
		} else if isAudioMime(mimeType) {
			msgType = "voice"
		}
		// FileSource carries the actual file path; MsgContent carries the display type+name.
		// MsgContent must NOT have filePath – that field doesn't exist in simplex MsgContent.
		// image/video require the "image" field; video/voice require "duration" field.
		composed.FileSource = &simplexclient.CryptoFile{FilePath: tmpPathToClean}
		caption := msg.Content.GetCaption()
		switch msgType {
		case "image":
			thumb := ffmpegThumbnailBase64(ctx, tmpPathToClean)
			composed.MsgContent = simplexclient.MakeMsgContentImage(caption, thumb)
		case "video":
			thumb := ffmpegThumbnailBase64(ctx, tmpPathToClean)
			duration := 0
			if info := msg.Content.GetInfo(); info != nil && info.Duration > 0 {
				duration = int(info.Duration / 1000)
			}
			composed.MsgContent = simplexclient.MakeMsgContentVideo(caption, thumb, duration)
		case "voice":
			duration := 0
			if info := msg.Content.GetInfo(); info != nil && info.Duration > 0 {
				duration = int(info.Duration / 1000)
			}
			composed.MsgContent = simplexclient.MakeMsgContentVoice(caption, duration)
		default:
			composed.MsgContent = simplexclient.MakeMsgContentFile(fileName)
		}
	}

	// For plain text messages containing a URL, fetch a link preview and upgrade
	// the message to a SimpleX "link" type so recipients see the preview card.
	if composed.FileSource == nil && composed.MsgContent.Type == "text" {
		if uri := extractFirstURL(composed.MsgContent.Text); uri != "" {
			zerolog.Ctx(ctx).Debug().Str("uri", uri).Msg("Fetching link preview for outgoing message")
			if preview := fetchLinkPreview(ctx, s.Main.linkPreviewClient, uri); preview != nil {
				composed.MsgContent = simplexclient.MakeMsgContentLink(composed.MsgContent.Text, preview)
			}
		}
	}

	var sent []simplexclient.AChatItem
	if composed.FileSource != nil {
		// Use the retry path for file sends — simplex-chat may drop the connection when
		// processing a file transfer, and we want to reconnect and retry automatically.
		sent, err = s.Client.SendMessagesRetryOnce(ctx, chatType, chatID, []simplexclient.ComposedMessage{composed})
	} else {
		sent, err = s.Client.SendMessages(chatType, chatID, []simplexclient.ComposedMessage{composed})
	}
	// Clean up the temp file after simplex-chat has processed it (response received).
	if tmpPathToClean != "" {
		os.Remove(tmpPathToClean)
	}
	if err != nil {
		return nil, bridgev2.WrapErrorInStatus(err).WithSendNotice(true)
	}
	if len(sent) == 0 {
		return nil, fmt.Errorf("no chat items returned after send")
	}
	item := sent[0]
	msgID := simplexid.MakeMessageID(item.ChatItem.Meta.ItemID)
	txnID := networkid.TransactionID(msgID)
	loginUserID, _ := simplexid.ParseUserLoginID(s.UserLogin.ID)

	// Register the message ID to be ignored when the async newChatItems echo arrives.
	// simplex-chat sends the message back as both a corrId response (already handled above)
	// and as a separate async event with no corrId. Without this, the echo would be
	// bridged as a duplicate message in the Matrix room.
	msg.AddPendingToIgnore(txnID)

	return &bridgev2.MatrixMessageResponse{
		DB: &database.Message{
			ID:        msgID,
			SenderID:  simplexid.MakeUserID(loginUserID),
			Timestamp: time.Now(),
			Metadata:  &simplexid.MessageMetadata{},
		},
		RemovePending: txnID,
	}, nil
}

func isImageMime(mime string) bool {
	switch mime {
	case "image/jpeg", "image/png", "image/gif", "image/webp":
		return true
	}
	return false
}

// HandleMatrixEdit edits an existing SimpleX message.
func (s *SimplexClient) HandleMatrixEdit(ctx context.Context, msg *bridgev2.MatrixEdit) error {
	if s.Client == nil {
		return bridgev2.ErrNotLoggedIn
	}
	chatType, chatID, err := simplexid.ParsePortalID(msg.Portal.ID)
	if err != nil {
		return fmt.Errorf("failed to parse portal ID: %w", err)
	}
	itemID, err := simplexid.ParseMessageID(msg.EditTarget.ID)
	if err != nil {
		return fmt.Errorf("failed to parse message ID: %w", err)
	}
	content := MatrixToSimplexMsgContent(msg.Content)
	_, err = s.Client.UpdateChatItem(chatType, chatID, itemID, content)
	if err != nil {
		return bridgev2.WrapErrorInStatus(err).WithSendNotice(true)
	}
	return nil
}

// PreHandleMatrixReaction prepares a reaction before sending.
func (s *SimplexClient) PreHandleMatrixReaction(ctx context.Context, msg *bridgev2.MatrixReaction) (bridgev2.MatrixReactionPreResponse, error) {
	loginUserID, _ := simplexid.ParseUserLoginID(s.UserLogin.ID)
	return bridgev2.MatrixReactionPreResponse{
		SenderID: simplexid.MakeUserID(loginUserID),
		EmojiID:  "",
		Emoji:    msg.Content.RelatesTo.Key,
	}, nil
}

// HandleMatrixReaction sends a reaction to SimpleX.
func (s *SimplexClient) HandleMatrixReaction(ctx context.Context, msg *bridgev2.MatrixReaction) (*database.Reaction, error) {
	if s.Client == nil {
		return nil, bridgev2.ErrNotLoggedIn
	}
	emoji, ok := normalizeEmojiForSimplex(msg.PreHandleResp.Emoji)
	if !ok {
		// SimpleX only supports 8 specific emojis — silently ignore unsupported ones.
		return &database.Reaction{}, nil
	}
	chatType, chatID, err := simplexid.ParsePortalID(msg.Portal.ID)
	if err != nil {
		return nil, fmt.Errorf("failed to parse portal ID: %w", err)
	}
	itemID, err := simplexid.ParseMessageID(msg.TargetMessage.ID)
	if err != nil {
		return nil, fmt.Errorf("failed to parse message ID: %w", err)
	}
	err = s.Client.ReactToChatItem(chatType, chatID, itemID, emoji, true)
	if err != nil {
		return nil, err
	}
	return &database.Reaction{}, nil
}

// HandleMatrixReactionRemove removes a reaction from SimpleX.
func (s *SimplexClient) HandleMatrixReactionRemove(ctx context.Context, msg *bridgev2.MatrixReactionRemove) error {
	if s.Client == nil {
		return bridgev2.ErrNotLoggedIn
	}
	chatType, chatID, err := simplexid.ParsePortalID(msg.Portal.ID)
	if err != nil {
		return fmt.Errorf("failed to parse portal ID: %w", err)
	}
	itemID, err := simplexid.ParseMessageID(msg.TargetReaction.MessageID)
	if err != nil {
		return fmt.Errorf("failed to parse message ID: %w", err)
	}
	return s.Client.ReactToChatItem(chatType, chatID, itemID, msg.TargetReaction.Emoji, false)
}

// HandleMatrixMessageRemove deletes a message from SimpleX.
func (s *SimplexClient) HandleMatrixMessageRemove(ctx context.Context, msg *bridgev2.MatrixMessageRemove) error {
	if s.Client == nil {
		return bridgev2.ErrNotLoggedIn
	}
	chatType, chatID, err := simplexid.ParsePortalID(msg.Portal.ID)
	if err != nil {
		return fmt.Errorf("failed to parse portal ID: %w", err)
	}
	itemID, err := simplexid.ParseMessageID(msg.TargetMessage.ID)
	if err != nil {
		return fmt.Errorf("failed to parse message ID: %w", err)
	}
	return s.Client.DeleteChatItem(chatType, chatID, itemID, simplexclient.DeleteModeBroadcast)
}

// ffmpegThumbnailBase64 generates a small JPEG thumbnail from a media file using
// ffmpeg and returns it as a base64 data URI. The thumbnail is kept tiny (max 64px)
// at low quality so the base64 fits within SimpleX's ~16KB message size limit.
// Returns empty string on failure.
func ffmpegThumbnailBase64(ctx context.Context, filePath string) string {
	log := zerolog.Ctx(ctx)
	thumbPath := filePath + ".thumb.jpg"
	defer os.Remove(thumbPath)

	// Extract a single frame, scale to max 256px, encode as low quality JPEG.
	// SimpleX has a ~16KB message size limit and the thumbnail is embedded as
	// base64 inside the JSON payload, so aim for ~6-10KB base64 (q:v 10).
	// A larger but compressed image gives a better preview than a tiny sharp one.
	cmd := exec.CommandContext(ctx, "ffmpeg",
		"-i", filePath,
		"-vframes", "1",
		"-vf", "scale='min(256,iw)':'min(256,ih)':force_original_aspect_ratio=decrease",
		"-q:v", "10",
		"-y",
		thumbPath,
	)
	if out, err := cmd.CombinedOutput(); err != nil {
		log.Warn().Err(err).Str("output", string(out)).Msg("ffmpeg thumbnail generation failed")
		return ""
	}

	thumbData, err := os.ReadFile(thumbPath)
	if err != nil || len(thumbData) == 0 {
		return ""
	}

	return "data:image/jpg;base64," + base64.StdEncoding.EncodeToString(thumbData)
}

var urlRe = regexp.MustCompile(`https?://[^\s"'<>]+`)

// extractFirstURL returns the first http/https URL found in text, or "".
func extractFirstURL(text string) string {
	return urlRe.FindString(text)
}

var (
	ogMetaRe   = regexp.MustCompile(`(?i)<meta[^>]+>`)
	propertyRe = regexp.MustCompile(`(?i)property=["'](og:[^"']+)["']`)
	contentRe  = regexp.MustCompile(`(?i)content=["']([^"']*)["']`)
	titleTagRe = regexp.MustCompile(`(?i)<title[^>]*>([^<]+)</title>`)
)

func extractOGTag(html, prop string) string {
	for _, tag := range ogMetaRe.FindAllString(html, -1) {
		m := propertyRe.FindStringSubmatch(tag)
		if m == nil || !strings.EqualFold(m[1], prop) {
			continue
		}
		c := contentRe.FindStringSubmatch(tag)
		if c != nil {
			return c[1]
		}
	}
	return ""
}

// fetchLinkPreview fetches the page at uri and extracts OG metadata plus a
// thumbnail image. Returns nil if no useful data could be retrieved.
func fetchLinkPreview(ctx context.Context, client *http.Client, uri string) *simplexclient.LinkPreview {
	ctx, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, "GET", uri, nil)
	if err != nil {
		return nil
	}
	req.Header.Set("User-Agent", "TelegramBot (like TwitterBot)")
	req.Header.Set("Accept", "text/html,application/xhtml+xml")

	resp, err := client.Do(req)
	if err != nil {
		return nil
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil
	}
	ct := resp.Header.Get("Content-Type")
	if !strings.Contains(ct, "text/html") && !strings.Contains(ct, "xhtml") {
		return nil
	}

	raw, err := io.ReadAll(io.LimitReader(resp.Body, 256*1024))
	if err != nil {
		return nil
	}
	page := string(raw)

	title := extractOGTag(page, "og:title")
	if title == "" {
		if m := titleTagRe.FindStringSubmatch(page); m != nil {
			title = strings.TrimSpace(m[1])
		}
	}
	if title == "" {
		return nil
	}

	preview := &simplexclient.LinkPreview{
		URI:         uri,
		Title:       title,
		Description: extractOGTag(page, "og:description"),
	}

	// Fetch the og:image and generate a thumbnail via ffmpeg.
	if imgURL := extractOGTag(page, "og:image"); imgURL != "" {
		if thumb := fetchURLThumbnailBase64(ctx, client, imgURL); thumb != "" {
			preview.Image = thumb
		}
	}

	return preview
}

// fetchURLThumbnailBase64 downloads an image URL, writes it to a temp file,
// and returns a base64 thumbnail the same way ffmpegThumbnailBase64 does.
func fetchURLThumbnailBase64(ctx context.Context, client *http.Client, imgURL string) string {
	req, err := http.NewRequestWithContext(ctx, "GET", imgURL, nil)
	if err != nil {
		return ""
	}
	req.Header.Set("User-Agent", "TelegramBot (like TwitterBot)")

	resp, err := client.Do(req)
	if err != nil {
		return ""
	}
	defer resp.Body.Close()

	data, err := io.ReadAll(io.LimitReader(resp.Body, 4*1024*1024))
	if err != nil || len(data) == 0 {
		return ""
	}

	tmp, err := os.CreateTemp("", "preview-img-*")
	if err != nil {
		return ""
	}
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath)

	if _, err = tmp.Write(data); err != nil {
		tmp.Close()
		return ""
	}
	tmp.Close()

	return ffmpegThumbnailBase64(ctx, tmpPath)
}
