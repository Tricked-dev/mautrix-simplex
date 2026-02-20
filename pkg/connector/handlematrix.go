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
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"time"

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
			composed.MsgContent = simplexclient.MakeMsgContentImage(caption, "")
		case "video":
			composed.MsgContent = simplexclient.MakeMsgContentVideo(caption, "", 0)
		case "voice":
			composed.MsgContent = simplexclient.MakeMsgContentVoice(caption, 0)
		default:
			composed.MsgContent = simplexclient.MakeMsgContentFile(fileName)
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
