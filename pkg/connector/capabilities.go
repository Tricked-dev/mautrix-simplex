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

	"maunium.net/go/mautrix/bridgev2"
	"maunium.net/go/mautrix/bridgev2/database"
	"maunium.net/go/mautrix/bridgev2/networkid"
	"maunium.net/go/mautrix/event"

	"go.mau.fi/mautrix-simplex/pkg/simplexid"
)

var simplexCaps = &event.RoomFeatures{
	ID: "fi.mau.simplex.capabilities.2024",

	Formatting: map[event.FormattingFeature]event.CapabilitySupportLevel{
		event.FmtBold:          event.CapLevelFullySupported,
		event.FmtItalic:        event.CapLevelFullySupported,
		event.FmtStrikethrough: event.CapLevelFullySupported,
		event.FmtInlineCode:    event.CapLevelFullySupported,
		event.FmtCodeBlock:     event.CapLevelFullySupported,
		event.FmtInlineLink:    event.CapLevelPartialSupport,
	},
	File: map[event.CapabilityMsgType]*event.FileFeatures{
		event.MsgImage: {
			MimeTypes: map[string]event.CapabilitySupportLevel{
				"image/jpeg": event.CapLevelFullySupported,
				"image/png":  event.CapLevelFullySupported,
				"image/gif":  event.CapLevelFullySupported,
				"image/webp": event.CapLevelFullySupported,
			},
			Caption: event.CapLevelFullySupported,
		},
		event.MsgVideo: {
			MimeTypes: map[string]event.CapabilitySupportLevel{
				"video/mp4":  event.CapLevelFullySupported,
				"video/webm": event.CapLevelFullySupported,
			},
			Caption: event.CapLevelFullySupported,
		},
		event.MsgAudio: {
			MimeTypes: map[string]event.CapabilitySupportLevel{
				"audio/mpeg": event.CapLevelFullySupported,
				"audio/aac":  event.CapLevelFullySupported,
				"audio/ogg":  event.CapLevelFullySupported,
			},
		},
		event.MsgFile: {
			MimeTypes: map[string]event.CapabilitySupportLevel{
				"*/*": event.CapLevelFullySupported,
			},
			Caption: event.CapLevelFullySupported,
		},
	},

	Reply:  event.CapLevelFullySupported,
	Edit:   event.CapLevelFullySupported,
	Delete: event.CapLevelFullySupported,

	Reaction:         event.CapLevelFullySupported,
	ReactionCount:    -1,
	AllowedReactions: nil, // all emoji allowed
}

var simplexCapsDM *event.RoomFeatures

func init() {
	simplexCapsDM = &event.RoomFeatures{}
	*simplexCapsDM = *simplexCaps
	simplexCapsDM.ID = "fi.mau.simplex.capabilities.2024+dm"
}

func (s *SimplexClient) GetCapabilities(ctx context.Context, portal *bridgev2.Portal) *event.RoomFeatures {
	if portal.RoomType == database.RoomTypeDM {
		return simplexCapsDM
	}
	return simplexCaps
}

var simplexGeneralCaps = &bridgev2.NetworkGeneralCapabilities{
	DisappearingMessages: false,
	AggressiveUpdateInfo: false,
	Provisioning: bridgev2.ProvisioningCapabilities{
		ResolveIdentifier: bridgev2.ResolveIdentifierCapabilities{
			CreateDM: false,
		},
	},
}

func (s *SimplexConnector) GetCapabilities() *bridgev2.NetworkGeneralCapabilities {
	return simplexGeneralCaps
}

func (s *SimplexConnector) GetBridgeInfoVersion() (info, capabilities int) {
	return 1, 1
}

// GetDBMetaTypes returns the metadata type instances for bridgev2 database.
func (s *SimplexConnector) GetDBMetaTypes() database.MetaTypes {
	return database.MetaTypes{
		Portal:    func() any { return &simplexid.PortalMetadata{} },
		Ghost:     func() any { return &simplexid.GhostMetadata{} },
		Message:   func() any { return &simplexid.MessageMetadata{} },
		Reaction:  nil,
		UserLogin: func() any { return &simplexid.UserLoginMetadata{} },
	}
}

// GetUserID returns the network user ID for the given user login.
func (s *SimplexConnector) GetUserID(loginID networkid.UserLoginID) networkid.UserID {
	return networkid.UserID(loginID)
}
