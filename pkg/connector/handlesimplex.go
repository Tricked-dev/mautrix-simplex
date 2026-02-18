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
	"encoding/json"
	"fmt"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/rs/zerolog"
	"maunium.net/go/mautrix/bridgev2"
	"maunium.net/go/mautrix/bridgev2/database"
	"maunium.net/go/mautrix/bridgev2/networkid"
	"maunium.net/go/mautrix/bridgev2/simplevent"
	"maunium.net/go/mautrix/event"

	"go.mau.fi/mautrix-simplex/pkg/simplexclient"
	"go.mau.fi/mautrix-simplex/pkg/simplexid"
)

// handleSimplexEvent routes incoming SimpleX events to the appropriate handler.
func (s *SimplexClient) handleSimplexEvent(ctx context.Context, evt simplexclient.Event) {
	log := zerolog.Ctx(ctx)
	switch evt.Type {
	case "newChatItems":
		var data simplexclient.NewChatItemsEvent
		if err := json.Unmarshal(evt.Raw, &data); err != nil {
			log.Err(err).Msg("Failed to unmarshal newChatItems event")
			return
		}
		s.handleNewChatItems(ctx, data)

	case "chatItemUpdated":
		var data simplexclient.ChatItemUpdatedEvent
		if err := json.Unmarshal(evt.Raw, &data); err != nil {
			log.Err(err).Msg("Failed to unmarshal chatItemUpdated event")
			return
		}
		s.handleChatItemUpdated(ctx, data)

	case "chatItemsDeleted":
		var data simplexclient.ChatItemsDeletedEvent
		if err := json.Unmarshal(evt.Raw, &data); err != nil {
			log.Err(err).Msg("Failed to unmarshal chatItemsDeleted event")
			return
		}
		s.handleChatItemsDeleted(ctx, data)

	case "chatItemReaction":
		var data simplexclient.ChatItemReactionEvent
		if err := json.Unmarshal(evt.Raw, &data); err != nil {
			log.Err(err).Msg("Failed to unmarshal chatItemReaction event")
			return
		}
		s.handleChatItemReaction(ctx, data)

	case "contactConnected":
		var data simplexclient.ContactConnectedEvent
		if err := json.Unmarshal(evt.Raw, &data); err != nil {
			log.Err(err).Msg("Failed to unmarshal contactConnected event")
			return
		}
		s.handleContactConnected(ctx, data)

	case "contactUpdated":
		var data simplexclient.ContactUpdatedEvent
		if err := json.Unmarshal(evt.Raw, &data); err != nil {
			log.Err(err).Msg("Failed to unmarshal contactUpdated event")
			return
		}
		s.handleContactUpdated(ctx, data)

	case "joinedGroupMember":
		var data simplexclient.JoinedGroupMemberEvent
		if err := json.Unmarshal(evt.Raw, &data); err != nil {
			log.Err(err).Msg("Failed to unmarshal joinedGroupMember event")
			return
		}
		s.handleJoinedGroupMember(ctx, data)

	case "deletedMember", "leftMember":
		var data simplexclient.DeletedMemberEvent
		if err := json.Unmarshal(evt.Raw, &data); err != nil {
			log.Err(err).Msg("Failed to unmarshal deletedMember/leftMember event")
			return
		}
		s.handleMemberLeft(ctx, data)

	case "groupUpdated":
		var data simplexclient.GroupUpdatedEvent
		if err := json.Unmarshal(evt.Raw, &data); err != nil {
			log.Err(err).Msg("Failed to unmarshal groupUpdated event")
			return
		}
		s.handleGroupUpdated(ctx, data)

	case "rcvFileComplete":
		var data simplexclient.RcvFileCompleteEvent
		if err := json.Unmarshal(evt.Raw, &data); err != nil {
			log.Err(err).Msg("Failed to unmarshal rcvFileComplete event")
			return
		}
		// Re-process the chat item now that the file is downloaded.
		s.handleNewChatItems(ctx, simplexclient.NewChatItemsEvent{
			User:      data.User,
			ChatItems: []simplexclient.AChatItem{data.ChatItem},
		})

	case "receivedContactRequest":
		var data simplexclient.ReceivedContactRequestEvent
		if err := json.Unmarshal(evt.Raw, &data); err != nil {
			log.Err(err).Msg("Failed to unmarshal receivedContactRequest event")
			return
		}
		s.handleReceivedContactRequest(ctx, data)

	default:
		log.Debug().Str("event_type", evt.Type).Msg("Unhandled SimpleX event type")
	}
}

// handleNewChatItems handles incoming messages.
func (s *SimplexClient) handleNewChatItems(ctx context.Context, data simplexclient.NewChatItemsEvent) {
	for _, aci := range data.ChatItems {
		item := aci.ChatItem
		portalKey := s.makePortalKeyFromChatInfo(aci.ChatInfo)
		sender := s.makeEventSenderFromDir(item.ChatDir)

		// Resolve directRcv sender: use contact from chat info
		if item.ChatDir.Type == "directRcv" && aci.ChatInfo.Contact != nil {
			sender = s.makeEventSenderFromContact(aci.ChatInfo.Contact)
		}

		ts := parseSimplexTime(item.Meta.CreatedAt)
		msgID := simplexid.MakeMessageID(item.Meta.ItemID)

		// For messages we sent ourselves, set TransactionID = msgID so that
		// AddPendingToIgnore (registered in HandleMatrixMessage) can suppress
		// the echo that simplex-chat pushes as an async event after every send.
		var txnID networkid.TransactionID
		if item.ChatDir.Type == "directSnd" || item.ChatDir.Type == "groupSnd" {
			txnID = networkid.TransactionID(msgID)
		}

		s.UserLogin.QueueRemoteEvent(&simplevent.Message[*simplexclient.ChatItem]{
			EventMeta: simplevent.EventMeta{
				Type: bridgev2.RemoteEventMessage,
				LogContext: func(c zerolog.Context) zerolog.Context {
					return c.Int64("item_id", item.Meta.ItemID)
				},
				PortalKey:    portalKey,
				CreatePortal: true,
				Sender:       sender,
				Timestamp:    ts,
			},
			Data:          &item,
			ID:            msgID,
			TransactionID: txnID,
			ConvertMessageFunc: func(ctx context.Context, portal *bridgev2.Portal, intent bridgev2.MatrixAPI, data *simplexclient.ChatItem) (*bridgev2.ConvertedMessage, error) {
				cm := convertChatItemToMatrix(data)
				// If a file part needs to be uploaded, do it now.
				for _, part := range cm.Parts {
					if filePath, ok := part.Extra["fi.mau.simplex.file_path"].(string); ok {
						delete(part.Extra, "fi.mau.simplex.file_path")
						if err := uploadFilePartToMatrix(ctx, portal, intent, part, filePath); err != nil {
							zerolog.Ctx(ctx).Err(err).Str("file_path", filePath).Msg("Failed to upload file to Matrix")
							part.Content = &event.MessageEventContent{
								MsgType: event.MsgNotice,
								Body:    "[File transfer failed: " + err.Error() + "]",
							}
						}
					}
				}
				return cm, nil
			},
		})
	}
}

// convertChatItemToMatrix converts a SimpleX ChatItem to a Matrix ConvertedMessage.
// When a file is available (FilePath set), the caller should pass a non-nil intent so
// the file can be uploaded to Matrix. If intent is nil, a notice is sent instead.
func convertChatItemToMatrix(item *simplexclient.ChatItem) *bridgev2.ConvertedMessage {
	body := item.Meta.ItemText
	var html string
	if len(item.FormattedText) > 0 {
		body, html = SimplexFormattedToMatrix(item.FormattedText)
	}

	if item.Meta.ItemDeleted != nil {
		return &bridgev2.ConvertedMessage{
			Parts: []*bridgev2.ConvertedMessagePart{{
				ID:   networkid.PartID(""),
				Type: event.EventMessage,
				Content: &event.MessageEventContent{
					MsgType: event.MsgNotice,
					Body:    "[Message deleted]",
				},
				Extra: map[string]any{},
			}},
		}
	}

	// If there is a file attached and it has been downloaded (FilePath set), convert it.
	if item.File != nil && item.File.FilePath != nil {
		return &bridgev2.ConvertedMessage{
			Parts: []*bridgev2.ConvertedMessagePart{{
				ID:   networkid.PartID("file"),
				Type: event.EventMessage,
				Content: &event.MessageEventContent{
					MsgType: event.MsgFile,
					Body:    item.File.FileName,
					Info: &event.FileInfo{
						Size: int(item.File.FileSize),
					},
				},
				Extra: map[string]any{
					"fi.mau.simplex.file_path": *item.File.FilePath,
				},
			}},
		}
	}

	content := &event.MessageEventContent{
		MsgType: event.MsgText,
		Body:    body,
	}
	if html != "" {
		content.Format = event.FormatHTML
		content.FormattedBody = html
	}

	return &bridgev2.ConvertedMessage{
		Parts: []*bridgev2.ConvertedMessagePart{{
			ID:      networkid.PartID(""),
			Type:    event.EventMessage,
			Content: content,
			Extra:   map[string]any{},
		}},
	}
}

// handleChatItemUpdated handles message edits.
func (s *SimplexClient) handleChatItemUpdated(ctx context.Context, data simplexclient.ChatItemUpdatedEvent) {
	item := data.ChatItem.ChatItem
	portalKey := s.makePortalKeyFromChatInfo(data.ChatItem.ChatInfo)
	sender := s.makeEventSenderFromDir(item.ChatDir)
	if item.ChatDir.Type == "directRcv" && data.ChatItem.ChatInfo.Contact != nil {
		sender = s.makeEventSenderFromContact(data.ChatItem.ChatInfo.Contact)
	}

	ts := parseSimplexTime(item.Meta.CreatedAt)
	msgID := simplexid.MakeMessageID(item.Meta.ItemID)

	s.UserLogin.QueueRemoteEvent(&simplevent.Message[*simplexclient.ChatItem]{
		EventMeta: simplevent.EventMeta{
			Type: bridgev2.RemoteEventEdit,
			LogContext: func(c zerolog.Context) zerolog.Context {
				return c.Int64("item_id", item.Meta.ItemID)
			},
			PortalKey: portalKey,
			Sender:    sender,
			Timestamp: ts,
		},
		TargetMessage: msgID,
		Data:          &item,
		ConvertEditFunc: func(ctx context.Context, portal *bridgev2.Portal, intent bridgev2.MatrixAPI, existing []*database.Message, data *simplexclient.ChatItem) (*bridgev2.ConvertedEdit, error) {
			cm := convertChatItemToMatrix(data)
			editParts := make([]*bridgev2.ConvertedEditPart, len(cm.Parts))
			for i, p := range cm.Parts {
				if filePath, ok := p.Extra["fi.mau.simplex.file_path"].(string); ok {
					delete(p.Extra, "fi.mau.simplex.file_path")
					if err := uploadFilePartToMatrix(ctx, portal, intent, p, filePath); err != nil {
						zerolog.Ctx(ctx).Err(err).Str("file_path", filePath).Msg("Failed to upload file to Matrix (edit)")
					}
				}
				editParts[i] = &bridgev2.ConvertedEditPart{
					Type:    p.Type,
					Content: p.Content,
					Extra:   p.Extra,
				}
			}
			return &bridgev2.ConvertedEdit{ModifiedParts: editParts}, nil
		},
	})
}

// handleChatItemsDeleted handles message deletions.
func (s *SimplexClient) handleChatItemsDeleted(ctx context.Context, data simplexclient.ChatItemsDeletedEvent) {
	for _, del := range data.ChatItemDeletions {
		if del.DeletedChatItem == nil {
			continue
		}
		item := del.DeletedChatItem.ChatItem
		portalKey := s.makePortalKeyFromChatInfo(del.DeletedChatItem.ChatInfo)
		msgID := simplexid.MakeMessageID(item.Meta.ItemID)

		s.UserLogin.QueueRemoteEvent(&simplevent.MessageRemove{
			EventMeta: simplevent.EventMeta{
				Type: bridgev2.RemoteEventMessageRemove,
				LogContext: func(c zerolog.Context) zerolog.Context {
					return c.Int64("item_id", item.Meta.ItemID)
				},
				PortalKey: portalKey,
				Sender:    s.makeEventSenderFromDir(item.ChatDir),
				Timestamp: time.Now(),
			},
			TargetMessage: msgID,
		})
	}
}

// handleChatItemReaction handles reactions add/remove.
func (s *SimplexClient) handleChatItemReaction(ctx context.Context, data simplexclient.ChatItemReactionEvent) {
	reaction := data.Reaction
	var sender bridgev2.EventSender
	if reaction.FromContact != nil {
		sender = s.makeEventSenderFromContact(reaction.FromContact)
	} else if reaction.FromMember != nil {
		sender = s.makeEventSenderFromMember(reaction.FromMember)
	} else {
		loginID, _ := simplexid.ParseUserLoginID(s.UserLogin.ID)
		sender = bridgev2.EventSender{IsFromMe: true, Sender: simplexid.MakeUserID(loginID)}
	}

	portalKey := s.makePortalKeyFromChatInfo(reaction.ChatInfo)
	// We don't have the target chat item ID in ACIReaction, so we can't queue a
	// proper reaction event without more data. Log and skip for now.
	zerolog.Ctx(ctx).Debug().
		Bool("added", data.Added).
		Str("emoji", reaction.Reaction.Emoji).
		Str("portal", string(portalKey.ID)).
		Str("sender", string(sender.Sender)).
		Msg("Received reaction event (not yet mapped to target message)")
}

// handleReceivedContactRequest auto-accepts incoming contact requests.
func (s *SimplexClient) handleReceivedContactRequest(ctx context.Context, data simplexclient.ReceivedContactRequestEvent) {
	log := zerolog.Ctx(ctx)
	req := data.ContactRequest
	log.Info().
		Int64("contact_req_id", req.ContactRequestID).
		Str("display_name", req.LocalDisplayName).
		Msg("Auto-accepting incoming contact request")

	contact, err := s.Client.AcceptContact(req.ContactRequestID)
	if err != nil {
		log.Err(err).Int64("contact_req_id", req.ContactRequestID).Msg("Failed to auto-accept contact request")
		return
	}

	// Create the DM portal for this newly accepted contact.
	portalKey := networkid.PortalKey{
		ID:       simplexid.MakeDMPortalID(contact.ContactID),
		Receiver: s.UserLogin.ID,
	}
	s.UserLogin.QueueRemoteEvent(&simplevent.ChatResync{
		EventMeta: simplevent.EventMeta{
			Type:         bridgev2.RemoteEventChatResync,
			PortalKey:    portalKey,
			CreatePortal: true,
		},
		GetChatInfoFunc: s.GetChatInfo,
	})
}

// handleContactConnected handles a new contact being connected.
func (s *SimplexClient) handleContactConnected(ctx context.Context, data simplexclient.ContactConnectedEvent) {
	contact := data.Contact
	portalKey := networkid.PortalKey{
		ID:       simplexid.MakeDMPortalID(contact.ContactID),
		Receiver: s.UserLogin.ID,
	}
	s.UserLogin.QueueRemoteEvent(&simplevent.ChatResync{
		EventMeta: simplevent.EventMeta{
			Type:         bridgev2.RemoteEventChatResync,
			PortalKey:    portalKey,
			CreatePortal: true,
		},
		GetChatInfoFunc: s.GetChatInfo,
	})
}

// handleContactUpdated handles a contact profile update.
func (s *SimplexClient) handleContactUpdated(ctx context.Context, data simplexclient.ContactUpdatedEvent) {
	contact := data.ToContact
	ghostID := simplexid.MakeUserID(contact.ContactID)
	info := s.contactToUserInfo(&contact)

	ghost, err := s.Main.Bridge.GetGhostByID(ctx, ghostID)
	if err != nil {
		zerolog.Ctx(ctx).Err(err).Msg("Failed to get ghost for contactUpdated")
		return
	}
	ghost.UpdateInfo(ctx, info)
}

// handleJoinedGroupMember handles a new member joining a group.
func (s *SimplexClient) handleJoinedGroupMember(ctx context.Context, data simplexclient.JoinedGroupMemberEvent) {
	portalKey := networkid.PortalKey{
		ID:       simplexid.MakeGroupPortalID(data.GroupInfo.GroupID),
		Receiver: s.UserLogin.ID,
	}
	s.UserLogin.QueueRemoteEvent(&simplevent.ChatResync{
		EventMeta: simplevent.EventMeta{
			Type:      bridgev2.RemoteEventChatResync,
			PortalKey: portalKey,
		},
		GetChatInfoFunc: s.GetChatInfo,
	})
}

// handleMemberLeft handles a member leaving or being removed from a group.
func (s *SimplexClient) handleMemberLeft(ctx context.Context, data simplexclient.DeletedMemberEvent) {
	portalKey := networkid.PortalKey{
		ID:       simplexid.MakeGroupPortalID(data.GroupInfo.GroupID),
		Receiver: s.UserLogin.ID,
	}
	s.UserLogin.QueueRemoteEvent(&simplevent.ChatResync{
		EventMeta: simplevent.EventMeta{
			Type:      bridgev2.RemoteEventChatResync,
			PortalKey: portalKey,
		},
		GetChatInfoFunc: s.GetChatInfo,
	})
}

// handleGroupUpdated handles a group profile update.
func (s *SimplexClient) handleGroupUpdated(ctx context.Context, data simplexclient.GroupUpdatedEvent) {
	portalKey := networkid.PortalKey{
		ID:       simplexid.MakeGroupPortalID(data.ToGroup.GroupID),
		Receiver: s.UserLogin.ID,
	}
	s.UserLogin.QueueRemoteEvent(&simplevent.ChatResync{
		EventMeta: simplevent.EventMeta{
			Type:      bridgev2.RemoteEventChatResync,
			PortalKey: portalKey,
		},
		GetChatInfoFunc: s.GetChatInfo,
	})
}

// syncChats creates/updates portals for all existing contacts and groups.
func (s *SimplexClient) syncChats(ctx context.Context) {
	log := zerolog.Ctx(ctx)
	if s.Client == nil {
		return
	}
	loginID, err := simplexid.ParseUserLoginID(s.UserLogin.ID)
	if err != nil {
		log.Err(err).Msg("Failed to parse user login ID during sync")
		return
	}

	contacts, err := s.Client.ListContacts(loginID)
	if err != nil {
		log.Err(err).Msg("Failed to list contacts during sync")
	} else {
		for _, contact := range contacts {
			portalKey := networkid.PortalKey{
				ID:       simplexid.MakeDMPortalID(contact.ContactID),
				Receiver: s.UserLogin.ID,
			}
			s.UserLogin.QueueRemoteEvent(&simplevent.ChatResync{
				EventMeta: simplevent.EventMeta{
					Type:         bridgev2.RemoteEventChatResync,
					PortalKey:    portalKey,
					CreatePortal: true,
				},
				GetChatInfoFunc: s.GetChatInfo,
			})
		}
	}

	groups, err := s.Client.ListGroups(loginID)
	if err != nil {
		log.Err(err).Msg("Failed to list groups during sync")
	} else {
		for _, group := range groups {
			portalKey := networkid.PortalKey{
				ID:       simplexid.MakeGroupPortalID(group.GroupID),
				Receiver: s.UserLogin.ID,
			}
			s.UserLogin.QueueRemoteEvent(&simplevent.ChatResync{
				EventMeta: simplevent.EventMeta{
					Type:         bridgev2.RemoteEventChatResync,
					PortalKey:    portalKey,
					CreatePortal: true,
				},
				GetChatInfoFunc: s.GetChatInfo,
			})
		}
	}

	// Mark chats as synced
	meta := s.UserLogin.Metadata.(*simplexid.UserLoginMetadata)
	meta.ChatsSynced = true
	if err := s.UserLogin.Save(ctx); err != nil {
		log.Err(err).Msg("Failed to save user login after chat sync")
	}
}

// parseSimplexTime parses a SimpleX timestamp string (RFC3339/ISO8601).
func parseSimplexTime(ts string) time.Time {
	t, err := time.Parse(time.RFC3339, ts)
	if err != nil {
		return time.Now()
	}
	return t
}

// uploadFilePartToMatrix reads a local file and uploads it to Matrix, updating the ConvertedMessagePart in place.
func uploadFilePartToMatrix(ctx context.Context, portal *bridgev2.Portal, intent bridgev2.MatrixAPI, part *bridgev2.ConvertedMessagePart, filePath string) error {
	data, err := os.ReadFile(filePath)
	if err != nil {
		return fmt.Errorf("read file: %w", err)
	}

	fileName := filepath.Base(filePath)
	if part.Content != nil && part.Content.Body != "" {
		fileName = part.Content.Body
	}

	mimeType := mime.TypeByExtension(filepath.Ext(fileName))
	if mimeType == "" {
		mimeType = http.DetectContentType(data)
	}

	uri, encFile, err := intent.UploadMedia(ctx, portal.MXID, data, fileName, mimeType)
	if err != nil {
		return fmt.Errorf("upload media: %w", err)
	}

	mc := part.Content
	if mc == nil {
		mc = &event.MessageEventContent{}
		part.Content = mc
	}

	// Set the MsgType based on mime type.
	switch {
	case isImageMime(mimeType):
		mc.MsgType = event.MsgImage
	case isVideoMime(mimeType):
		mc.MsgType = event.MsgVideo
	case isAudioMime(mimeType):
		mc.MsgType = event.MsgAudio
	default:
		mc.MsgType = event.MsgFile
	}
	mc.Body = fileName
	if mc.Info == nil {
		mc.Info = &event.FileInfo{}
	}
	mc.Info.MimeType = mimeType
	mc.Info.Size = len(data)
	mc.URL = uri
	mc.File = encFile

	return nil
}

func isVideoMime(mime string) bool {
	switch mime {
	case "video/mp4", "video/webm", "video/ogg":
		return true
	}
	return false
}

func isAudioMime(mime string) bool {
	switch mime {
	case "audio/mpeg", "audio/ogg", "audio/aac", "audio/wav":
		return true
	}
	return false
}
