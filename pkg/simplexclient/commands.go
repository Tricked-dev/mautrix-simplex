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

package simplexclient

import (
	"context"
	"encoding/json"
	"fmt"
)

// chatRef returns the text chat reference used in commands, e.g. "@42" or "#7"
func chatRef(chatType ChatType, chatID int64) string {
	return fmt.Sprintf("%s%d", chatType, chatID)
}

// GetActiveUser retrieves the active user profile
func (c *Client) GetActiveUser() (*User, error) {
	respType, raw, err := c.sendCmd(`/u`)
	if err != nil {
		return nil, err
	}
	switch respType {
	case "activeUser":
		var r struct {
			User User `json:"user"`
		}
		if err := json.Unmarshal(raw, &r); err != nil {
			return nil, fmt.Errorf("failed to parse activeUser response: %w", err)
		}
		return &r.User, nil
	default:
		return nil, fmt.Errorf("unexpected response type: %s (raw: %s)", respType, string(raw))
	}
}

// ListContacts retrieves all contacts for the given user
func (c *Client) ListContacts(userID int64) ([]Contact, error) {
	cmd := fmt.Sprintf("/_contacts %d", userID)
	respType, raw, err := c.sendCmd(cmd)
	if err != nil {
		return nil, err
	}
	if respType != "contactsList" {
		return nil, fmt.Errorf("unexpected response type: %s", respType)
	}
	var r struct {
		Contacts []Contact `json:"contacts"`
	}
	if err := json.Unmarshal(raw, &r); err != nil {
		return nil, fmt.Errorf("failed to parse contactsList: %w", err)
	}
	return r.Contacts, nil
}

// ListGroups retrieves all groups for the given user
func (c *Client) ListGroups(userID int64) ([]GroupInfo, error) {
	// Note: no space between /_groups and the userId
	cmd := fmt.Sprintf("/_groups%d", userID)
	respType, raw, err := c.sendCmd(cmd)
	if err != nil {
		return nil, err
	}
	if respType != "groupsList" {
		return nil, fmt.Errorf("unexpected response type: %s", respType)
	}
	var r struct {
		Groups []GroupInfo `json:"groups"`
	}
	if err := json.Unmarshal(raw, &r); err != nil {
		return nil, fmt.Errorf("failed to parse groupsList: %w", err)
	}
	return r.Groups, nil
}

// ListMembers retrieves members of a group
func (c *Client) ListMembers(groupID int64) ([]GroupMember, error) {
	cmd := fmt.Sprintf("/_members #%d", groupID)
	respType, raw, err := c.sendCmd(cmd)
	if err != nil {
		return nil, err
	}
	if respType != "groupMembers" {
		return nil, fmt.Errorf("unexpected response type: %s", respType)
	}
	var r struct {
		Group struct {
			Members []GroupMember `json:"members"`
		} `json:"group"`
	}
	if err := json.Unmarshal(raw, &r); err != nil {
		return nil, fmt.Errorf("failed to parse groupMembers: %w", err)
	}
	return r.Group.Members, nil
}

// GetChat retrieves chat messages with pagination
func (c *Client) GetChat(chatType ChatType, chatID int64, pagination ChatPagination) (*AChat, error) {
	var paginationStr string
	switch pagination.Type {
	case PaginationLast:
		paginationStr = fmt.Sprintf("count=%d", pagination.Count)
	case PaginationBefore:
		paginationStr = fmt.Sprintf("before=%d count=%d", pagination.ItemID, pagination.Count)
	case PaginationAfter:
		paginationStr = fmt.Sprintf("after=%d count=%d", pagination.ItemID, pagination.Count)
	case PaginationAround:
		paginationStr = fmt.Sprintf("around=%d count=%d", pagination.ItemID, pagination.Count)
	case PaginationInitial:
		paginationStr = fmt.Sprintf("initial=%d", pagination.Count)
	default:
		paginationStr = fmt.Sprintf("count=%d", pagination.Count)
	}
	cmd := fmt.Sprintf("/_get chat %s%d %s", chatType, chatID, paginationStr)
	respType, raw, err := c.sendCmd(cmd)
	if err != nil {
		return nil, err
	}
	if respType != "apiChat" {
		return nil, fmt.Errorf("unexpected response type: %s", respType)
	}
	var r struct {
		Chat AChat `json:"chat"`
	}
	if err := json.Unmarshal(raw, &r); err != nil {
		return nil, fmt.Errorf("failed to parse apiChat: %w", err)
	}
	return &r.Chat, nil
}

// SendMessages sends messages to a contact or group
func (c *Client) SendMessages(chatType ChatType, chatID int64, msgs []ComposedMessage) ([]AChatItem, error) {
	msgsJSON, err := json.Marshal(msgs)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal messages: %w", err)
	}
	// Format: /_send @<id> live=off json [<composedMessages>]
	cmd := fmt.Sprintf("/_send %s%d live=off json %s", chatType, chatID, msgsJSON)
	c.log.Debug().Str("send_cmd_preview", cmd[:min(len(cmd), 400)]).Msg("SendMessages command")
	respType, raw, err := c.sendCmd(cmd)
	if err != nil {
		return nil, err
	}
	if respType != "newChatItems" {
		return nil, fmt.Errorf("unexpected response type: %s", respType)
	}
	var r struct {
		ChatItems []AChatItem `json:"chatItems"`
	}
	if err := json.Unmarshal(raw, &r); err != nil {
		return nil, fmt.Errorf("failed to parse newChatItems: %w", err)
	}
	return r.ChatItems, nil
}

// SendMessagesRetryOnce sends messages like SendMessages but reconnects and retries once on
// connection loss. Use this for file/media sends where simplex-chat may drop the connection.
func (c *Client) SendMessagesRetryOnce(ctx context.Context, chatType ChatType, chatID int64, msgs []ComposedMessage) ([]AChatItem, error) {
	msgsJSON, err := json.Marshal(msgs)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal messages: %w", err)
	}
	cmd := fmt.Sprintf("/_send %s%d live=off json %s", chatType, chatID, msgsJSON)
	c.log.Debug().Str("send_cmd_preview", cmd[:min(len(cmd), 400)]).Msg("SendMessagesRetryOnce command")
	respType, raw, err := c.sendCmdRetryOnce(ctx, cmd)
	if err != nil {
		return nil, err
	}
	if respType != "newChatItems" {
		return nil, fmt.Errorf("unexpected response type: %s", respType)
	}
	var r struct {
		ChatItems []AChatItem `json:"chatItems"`
	}
	if err := json.Unmarshal(raw, &r); err != nil {
		return nil, fmt.Errorf("failed to parse newChatItems: %w", err)
	}
	return r.ChatItems, nil
}

// UpdateChatItem edits a message
func (c *Client) UpdateChatItem(chatType ChatType, chatID, itemID int64, content MsgContent) (*ChatItem, error) {
	updatedMsg := struct {
		MsgContent MsgContent        `json:"msgContent"`
		Mentions   map[string]string `json:"mentions"`
	}{
		MsgContent: content,
		Mentions:   map[string]string{},
	}
	updatedJSON, err := json.Marshal(updatedMsg)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal updated message: %w", err)
	}
	// Format: /_update item @<id> <itemId> live=off json<updatedMessage>
	cmd := fmt.Sprintf("/_update item %s%d %d live=off json%s", chatType, chatID, itemID, updatedJSON)
	respType, raw, err := c.sendCmd(cmd)
	if err != nil {
		return nil, err
	}
	if respType != "chatItemUpdated" {
		return nil, fmt.Errorf("unexpected response type: %s", respType)
	}
	var r struct {
		ChatItem AChatItem `json:"chatItem"`
	}
	if err := json.Unmarshal(raw, &r); err != nil {
		return nil, fmt.Errorf("failed to parse chatItemUpdated: %w", err)
	}
	return &r.ChatItem.ChatItem, nil
}

// DeleteChatItem deletes a message
func (c *Client) DeleteChatItem(chatType ChatType, chatID, itemID int64, mode DeleteMode) error {
	var modeStr string
	switch mode {
	case DeleteModeBroadcast:
		modeStr = "broadcast"
	case DeleteModeInternal:
		modeStr = "internal"
	default:
		modeStr = "broadcast"
	}
	// Format: /_delete item @<chatId> [<itemId>] <mode>
	// _strP parses a JSON value; NonEmpty ChatItemId and CIDeleteMode are both JSON-encoded
	itemIDsJSON := fmt.Sprintf("[%d]", itemID)
	deleteModeJSON := fmt.Sprintf(`{"type":"%s"}`, modeStr)
	cmd := fmt.Sprintf("/_delete item %s%d %s %s", chatType, chatID, itemIDsJSON, deleteModeJSON)
	respType, _, err := c.sendCmd(cmd)
	if err != nil {
		return err
	}
	if respType != "chatItemsDeleted" {
		return fmt.Errorf("unexpected response type: %s", respType)
	}
	return nil
}

// ReactToChatItem adds or removes a reaction
func (c *Client) ReactToChatItem(chatType ChatType, chatID, itemID int64, emoji string, add bool) error {
	addStr := "on"
	if !add {
		addStr = "off"
	}
	reactionJSON, _ := json.Marshal(map[string]string{"type": "emoji", "emoji": emoji})
	// Format: /_reaction @<chatId> <itemId> on/off <reactionJSON>
	cmd := fmt.Sprintf("/_reaction %s%d %d %s %s", chatType, chatID, itemID, addStr, reactionJSON)
	respType, _, err := c.sendCmd(cmd)
	if err != nil {
		return err
	}
	if respType != "chatItemReaction" {
		return fmt.Errorf("unexpected response type: %s", respType)
	}
	return nil
}

// AcceptContact accepts an incoming contact request
func (c *Client) AcceptContact(contactReqID int64) (*Contact, error) {
	// Format: /_accept incognito=off <contactReqId>
	cmd := fmt.Sprintf("/_accept incognito=off %d", contactReqID)
	respType, raw, err := c.sendCmd(cmd)
	if err != nil {
		return nil, err
	}
	if respType != "acceptingContactRequest" {
		return nil, fmt.Errorf("unexpected response type: %s", respType)
	}
	var r struct {
		Contact Contact `json:"contact"`
	}
	if err := json.Unmarshal(raw, &r); err != nil {
		return nil, fmt.Errorf("failed to parse acceptingContactRequest: %w", err)
	}
	return &r.Contact, nil
}

// CreateAddress creates a SimpleX address for the user
func (c *Client) CreateAddress(userID int64) (string, error) {
	// Format: /_address <userId>
	cmd := fmt.Sprintf("/_address %d", userID)
	respType, raw, err := c.sendCmd(cmd)
	if err != nil {
		return "", err
	}
	if respType != "userContactLinkCreated" {
		return "", fmt.Errorf("unexpected response type: %s", respType)
	}
	var r struct {
		ConnLinkContact struct {
			ConnShortLink string `json:"connShortLink"`
			ConnFullLink  string `json:"connFullLink"`
		} `json:"connLinkContact"`
	}
	if err := json.Unmarshal(raw, &r); err != nil {
		return "", fmt.Errorf("failed to parse userContactLinkCreated: %w", err)
	}
	if r.ConnLinkContact.ConnShortLink != "" {
		return r.ConnLinkContact.ConnShortLink, nil
	}
	return r.ConnLinkContact.ConnFullLink, nil
}

// SetAddressAutoAccept configures auto-accept for contact requests
func (c *Client) SetAddressAutoAccept(userID int64, autoAccept bool, autoReply *MsgContent) error {
	var settingsJSON []byte
	var err error
	if autoAccept {
		if autoReply != nil {
			replyJSON, _ := json.Marshal(autoReply)
			settingsJSON = []byte(fmt.Sprintf(`{"businessAddress":false,"autoAccept":{"acceptIncognito":false},"autoReply":%s}`, replyJSON))
		} else {
			settingsJSON = []byte(`{"businessAddress":false,"autoAccept":{"acceptIncognito":false}}`)
		}
	} else {
		settingsJSON = []byte(`{"businessAddress":false}`)
	}
	// Format: /_address_settings <userId> <settingsJSON>
	cmd := fmt.Sprintf("/_address_settings %d %s", userID, settingsJSON)
	respType, _, err := c.sendCmd(cmd)
	if err != nil {
		return err
	}
	if respType != "userContactLinkUpdated" {
		return fmt.Errorf("unexpected response type: %s", respType)
	}
	return nil
}

// JoinGroup accepts a group invitation
func (c *Client) JoinGroup(groupID int64) (*GroupInfo, error) {
	// Format: /_join #<groupId>
	cmd := fmt.Sprintf("/_join #%d", groupID)
	respType, raw, err := c.sendCmd(cmd)
	if err != nil {
		return nil, err
	}
	if respType != "userAcceptedGroupSent" {
		return nil, fmt.Errorf("unexpected response type: %s", respType)
	}
	var r struct {
		GroupInfo GroupInfo `json:"groupInfo"`
	}
	if err := json.Unmarshal(raw, &r); err != nil {
		return nil, fmt.Errorf("failed to parse userAcceptedGroupSent: %w", err)
	}
	return &r.GroupInfo, nil
}

// ReceiveFile accepts and starts downloading a file
func (c *Client) ReceiveFile(fileID int64) error {
	cmd := fmt.Sprintf("/freceive %d approved_relays=on", fileID)
	respType, _, err := c.sendCmd(cmd)
	if err != nil {
		return err
	}
	if respType != "rcvFileAccepted" && respType != "rcvFileAcceptedSndCancelled" {
		return fmt.Errorf("unexpected response type: %s", respType)
	}
	return nil
}

// UpdateGroupProfile updates a group's profile
func (c *Client) UpdateGroupProfile(groupID int64, profile GroupProfile) (*GroupInfo, error) {
	profileJSON, err := json.Marshal(profile)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal group profile: %w", err)
	}
	// Format: /_group_profile #<groupId> <profileJSON>
	cmd := fmt.Sprintf("/_group_profile #%d %s", groupID, profileJSON)
	respType, raw, err := c.sendCmd(cmd)
	if err != nil {
		return nil, err
	}
	if respType != "groupUpdated" {
		return nil, fmt.Errorf("unexpected response type: %s", respType)
	}
	var r struct {
		ToGroup GroupInfo `json:"toGroup"`
	}
	if err := json.Unmarshal(raw, &r); err != nil {
		return nil, fmt.Errorf("failed to parse groupUpdated: %w", err)
	}
	return &r.ToGroup, nil
}
