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

package simplexid

import (
	"fmt"
	"strconv"
	"strings"

	"maunium.net/go/mautrix/bridgev2/networkid"

	"go.mau.fi/mautrix-simplex/pkg/simplexclient"
)

// MakeGroupPortalID creates a portal ID for a group chat.
// Format: "g:<groupId>"
func MakeGroupPortalID(groupID int64) networkid.PortalID {
	return networkid.PortalID(fmt.Sprintf("g:%d", groupID))
}

// MakeDMPortalID creates a portal ID for a direct message chat.
// Format: "d:<contactId>"
func MakeDMPortalID(contactID int64) networkid.PortalID {
	return networkid.PortalID(fmt.Sprintf("d:%d", contactID))
}

// ParsePortalID parses a portal ID and returns the chat type and ID.
func ParsePortalID(portalID networkid.PortalID) (simplexclient.ChatType, int64, error) {
	s := string(portalID)
	if strings.HasPrefix(s, "g:") {
		id, err := strconv.ParseInt(s[2:], 10, 64)
		if err != nil {
			return "", 0, fmt.Errorf("invalid group portal ID %q: %w", s, err)
		}
		return simplexclient.ChatTypeGroup, id, nil
	} else if strings.HasPrefix(s, "d:") {
		id, err := strconv.ParseInt(s[2:], 10, 64)
		if err != nil {
			return "", 0, fmt.Errorf("invalid DM portal ID %q: %w", s, err)
		}
		return simplexclient.ChatTypeDirect, id, nil
	}
	return "", 0, fmt.Errorf("unknown portal ID format: %q", s)
}

// ParsePortalToChatRef converts a portal ID into a simplexclient.ChatRef.
func ParsePortalToChatRef(portalID networkid.PortalID) (simplexclient.ChatRef, error) {
	chatType, chatID, err := ParsePortalID(portalID)
	if err != nil {
		return simplexclient.ChatRef{}, err
	}
	return simplexclient.ChatRef{ChatType: chatType, ChatID: chatID}, nil
}

// MakeUserID creates a user ID from a contact ID.
// Format: "<contactId>"
func MakeUserID(contactID int64) networkid.UserID {
	return networkid.UserID(fmt.Sprintf("%d", contactID))
}

// MakeMemberUserID creates a user ID from a member ID (hex-encoded bytes).
// Format: "m:<member_id_hex>"
func MakeMemberUserID(memberID string) networkid.UserID {
	return networkid.UserID(fmt.Sprintf("m:%s", memberID))
}

// ParseUserID parses a user ID and returns the contact ID.
// Returns -1 if the ID is a member ID (starts with "m:").
func ParseUserID(userID networkid.UserID) (int64, error) {
	s := string(userID)
	if strings.HasPrefix(s, "m:") {
		return -1, nil
	}
	id, err := strconv.ParseInt(s, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("invalid user ID %q: %w", s, err)
	}
	return id, nil
}

// MakeMessageID creates a message ID from a chat item ID.
func MakeMessageID(chatItemID int64) networkid.MessageID {
	return networkid.MessageID(fmt.Sprintf("%d", chatItemID))
}

// ParseMessageID parses a message ID and returns the chat item ID.
func ParseMessageID(messageID networkid.MessageID) (int64, error) {
	id, err := strconv.ParseInt(string(messageID), 10, 64)
	if err != nil {
		return 0, fmt.Errorf("invalid message ID %q: %w", messageID, err)
	}
	return id, nil
}

// MakeUserLoginID creates a user login ID from a SimpleX user ID.
func MakeUserLoginID(userID int64) networkid.UserLoginID {
	return networkid.UserLoginID(fmt.Sprintf("%d", userID))
}

// ParseUserLoginID parses a user login ID and returns the SimpleX user ID.
func ParseUserLoginID(userLoginID networkid.UserLoginID) (int64, error) {
	id, err := strconv.ParseInt(string(userLoginID), 10, 64)
	if err != nil {
		return 0, fmt.Errorf("invalid user login ID %q: %w", userLoginID, err)
	}
	return id, nil
}
