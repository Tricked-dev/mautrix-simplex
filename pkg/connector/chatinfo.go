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
	"strings"
	"time"

	"go.mau.fi/util/ptr"
	"maunium.net/go/mautrix/bridgev2"
	"maunium.net/go/mautrix/bridgev2/database"
	"maunium.net/go/mautrix/bridgev2/networkid"
	"maunium.net/go/mautrix/event"

	"go.mau.fi/mautrix-simplex/pkg/simplexclient"
	"go.mau.fi/mautrix-simplex/pkg/simplexid"
)

// GetChatInfo implements bridgev2.NetworkAPI.
func (s *SimplexClient) GetChatInfo(ctx context.Context, portal *bridgev2.Portal) (*bridgev2.ChatInfo, error) {
	chatType, chatID, err := simplexid.ParsePortalID(portal.ID)
	if err != nil {
		return nil, fmt.Errorf("failed to parse portal ID: %w", err)
	}
	if chatType == simplexclient.ChatTypeGroup {
		return s.getGroupChatInfo(ctx, chatID)
	}
	return s.getDMChatInfo(ctx, chatID)
}

func (s *SimplexClient) getDMChatInfo(ctx context.Context, contactID int64) (*bridgev2.ChatInfo, error) {
	if s.Client == nil {
		return nil, bridgev2.ErrNotLoggedIn
	}
	// Fetch contacts to find this one
	loginID, err := simplexid.ParseUserLoginID(s.UserLogin.ID)
	if err != nil {
		return nil, err
	}
	contacts, err := s.Client.ListContacts(loginID)
	if err != nil {
		return nil, fmt.Errorf("failed to list contacts: %w", err)
	}
	var contact *simplexclient.Contact
	for i := range contacts {
		if contacts[i].ContactID == contactID {
			contact = &contacts[i]
			break
		}
	}
	if contact == nil {
		return nil, fmt.Errorf("contact %d not found", contactID)
	}
	return s.contactToChatInfo(contact, loginID), nil
}

func (s *SimplexClient) getGroupChatInfo(ctx context.Context, groupID int64) (*bridgev2.ChatInfo, error) {
	if s.Client == nil {
		return nil, bridgev2.ErrNotLoggedIn
	}
	loginID, err := simplexid.ParseUserLoginID(s.UserLogin.ID)
	if err != nil {
		return nil, err
	}
	groups, err := s.Client.ListGroups(loginID)
	if err != nil {
		return nil, fmt.Errorf("failed to list groups: %w", err)
	}
	var group *simplexclient.GroupInfo
	for i := range groups {
		if groups[i].GroupID == groupID {
			group = &groups[i]
			break
		}
	}
	if group == nil {
		return nil, fmt.Errorf("group %d not found", groupID)
	}
	members, err := s.Client.ListMembers(groupID)
	if err != nil {
		return nil, fmt.Errorf("failed to list members: %w", err)
	}
	return s.groupToChatInfo(group, members, loginID), nil
}

func (s *SimplexClient) contactToChatInfo(contact *simplexclient.Contact, selfLoginID int64) *bridgev2.ChatInfo {
	name := contact.Profile.DisplayName
	if name == "" {
		name = contact.LocalDisplayName
	}
	otherUserID := simplexid.MakeUserID(contact.ContactID)
	selfUserID := simplexid.MakeUserID(selfLoginID)
	members := &bridgev2.ChatMemberList{
		IsFull: true,
		MemberMap: map[networkid.UserID]bridgev2.ChatMember{
			otherUserID: {
				EventSender: bridgev2.EventSender{Sender: otherUserID},
				Membership:  event.MembershipJoin,
			},
			selfUserID: {
				EventSender: bridgev2.EventSender{Sender: selfUserID, IsFromMe: true},
				Membership:  event.MembershipJoin,
			},
		},
		OtherUserID: otherUserID,
	}
	topic := "SimpleX DM"
	return &bridgev2.ChatInfo{
		Name:    &name,
		Topic:   &topic,
		Members: members,
		Type:    ptr.Ptr(database.RoomTypeDM),
		ExtraUpdates: func(ctx context.Context, portal *bridgev2.Portal) (changed bool) {
			meta := portal.Metadata.(*simplexid.PortalMetadata)
			if meta.LastSync.IsZero() {
				meta.LastSync.Time = time.Now()
				return true
			}
			return false
		},
	}
}

func (s *SimplexClient) groupToChatInfo(group *simplexclient.GroupInfo, members []simplexclient.GroupMember, selfLoginID int64) *bridgev2.ChatInfo {
	name := group.GroupProfile.DisplayName
	if name == "" {
		name = group.LocalDisplayName
	}
	topic := ""
	if group.GroupProfile.Description != nil {
		topic = *group.GroupProfile.Description
	}

	memberMap := make(map[networkid.UserID]bridgev2.ChatMember, len(members)+1)
	for _, m := range members {
		if m.MemberStatus != "memActive" && m.MemberStatus != "memCreator" && m.MemberStatus != "memAdmin" {
			continue
		}
		var userID networkid.UserID
		if m.ContactID != nil {
			userID = simplexid.MakeUserID(*m.ContactID)
		} else {
			userID = simplexid.MakeMemberUserID(m.MemberID)
		}
		pl := 0
		if m.MemberRole == simplexclient.GroupMemberRoleAdmin || m.MemberRole == simplexclient.GroupMemberRoleOwner {
			pl = 50
		}
		memberMap[userID] = bridgev2.ChatMember{
			EventSender: bridgev2.EventSender{Sender: userID},
			Membership:  event.MembershipJoin,
			PowerLevel:  &pl,
		}
	}
	// Add the local (self) user so the bridge invites @testuser to the room.
	selfUserID := simplexid.MakeUserID(selfLoginID)
	selfPL := 50
	memberMap[selfUserID] = bridgev2.ChatMember{
		EventSender: bridgev2.EventSender{Sender: selfUserID, IsFromMe: true},
		Membership:  event.MembershipJoin,
		PowerLevel:  &selfPL,
	}

	chatMembers := &bridgev2.ChatMemberList{
		IsFull:    true,
		MemberMap: memberMap,
	}

	return &bridgev2.ChatInfo{
		Name:    &name,
		Topic:   &topic,
		Members: chatMembers,
		Type:    ptr.Ptr(database.RoomTypeDefault),
		ExtraUpdates: func(ctx context.Context, portal *bridgev2.Portal) (changed bool) {
			meta := portal.Metadata.(*simplexid.PortalMetadata)
			if meta.LastSync.IsZero() {
				meta.LastSync.Time = time.Now()
				return true
			}
			return false
		},
	}
}

// GetUserInfo implements bridgev2.NetworkAPI.
func (s *SimplexClient) GetUserInfo(ctx context.Context, ghost *bridgev2.Ghost) (*bridgev2.UserInfo, error) {
	contactID, err := simplexid.ParseUserID(ghost.ID)
	if err != nil {
		return nil, fmt.Errorf("failed to parse user ID: %w", err)
	}
	if contactID == -1 {
		// Member-only ID, no contact info available
		return &bridgev2.UserInfo{}, nil
	}
	if s.Client == nil {
		return nil, bridgev2.ErrNotLoggedIn
	}
	loginID, err := simplexid.ParseUserLoginID(s.UserLogin.ID)
	if err != nil {
		return nil, err
	}
	contacts, err := s.Client.ListContacts(loginID)
	if err != nil {
		return nil, fmt.Errorf("failed to list contacts: %w", err)
	}
	for _, contact := range contacts {
		if contact.ContactID == contactID {
			return s.contactToUserInfo(&contact), nil
		}
	}
	return &bridgev2.UserInfo{}, nil
}

func (s *SimplexClient) contactToUserInfo(contact *simplexclient.Contact) *bridgev2.UserInfo {
	name := contact.Profile.DisplayName
	if name == "" {
		name = contact.LocalDisplayName
	}
	isBot := false
	ui := &bridgev2.UserInfo{
		Name:  &name,
		IsBot: &isBot,
	}
	if contact.Profile.Image != nil && *contact.Profile.Image != "" {
		imageData := *contact.Profile.Image
		avatarID := networkid.AvatarID("contact:" + fmt.Sprintf("%d", contact.ContactID))
		ui.Avatar = &bridgev2.Avatar{
			ID: avatarID,
			Get: func(ctx context.Context) ([]byte, error) {
				return decodeDataURI(imageData)
			},
		}
	}
	return ui
}

// decodeDataURI decodes a base64 data URI (e.g. "data:image/jpg;base64,...") into raw bytes.
func decodeDataURI(dataURI string) ([]byte, error) {
	if !strings.HasPrefix(dataURI, "data:") {
		return nil, fmt.Errorf("not a data URI")
	}
	comma := strings.Index(dataURI, ",")
	if comma < 0 {
		return nil, fmt.Errorf("malformed data URI: no comma")
	}
	encoded := dataURI[comma+1:]
	data, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		// Try URL-safe base64
		data, err = base64.URLEncoding.DecodeString(encoded)
		if err != nil {
			return nil, fmt.Errorf("base64 decode: %w", err)
		}
	}
	return data, nil
}

func (s *SimplexClient) memberToUserInfo(member *simplexclient.GroupMember) *bridgev2.UserInfo {
	name := member.Profile.DisplayName
	if name == "" {
		name = member.LocalDisplayName
	}
	isBot := false
	return &bridgev2.UserInfo{
		Name:  &name,
		IsBot: &isBot,
	}
}
