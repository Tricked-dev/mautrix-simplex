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
	"time"

	"github.com/rs/zerolog"
	"maunium.net/go/mautrix/bridgev2"
	"maunium.net/go/mautrix/bridgev2/networkid"

	"go.mau.fi/mautrix-simplex/pkg/simplexclient"
	"go.mau.fi/mautrix-simplex/pkg/simplexid"
)

var _ bridgev2.BackfillingNetworkAPI = (*SimplexClient)(nil)

// FetchMessages implements bridgev2.BackfillingNetworkAPI.
func (s *SimplexClient) FetchMessages(ctx context.Context, params bridgev2.FetchMessagesParams) (*bridgev2.FetchMessagesResponse, error) {
	if !s.IsLoggedIn() {
		return nil, bridgev2.ErrNotLoggedIn
	}
	chatType, chatID, err := simplexid.ParsePortalID(params.Portal.ID)
	if err != nil {
		return nil, fmt.Errorf("failed to parse portal ID: %w", err)
	}

	var pagination simplexclient.ChatPagination
	if params.AnchorMessage != nil {
		anchorItemID, err := simplexid.ParseMessageID(params.AnchorMessage.ID)
		if err != nil {
			return nil, fmt.Errorf("failed to parse anchor message ID: %w", err)
		}
		pagination = simplexclient.ChatPagination{
			Type:   simplexclient.PaginationBefore,
			ItemID: anchorItemID,
			Count:  params.Count,
		}
	} else {
		pagination = simplexclient.ChatPagination{
			Type:  simplexclient.PaginationLast,
			Count: params.Count,
		}
	}

	chat, err := s.Client.GetChat(chatType, chatID, pagination)
	if err != nil {
		return nil, fmt.Errorf("failed to get chat: %w", err)
	}
	if chat == nil {
		zerolog.Ctx(ctx).Debug().Msg("GetChat returned nil, no messages to backfill")
		return nil, nil
	}

	convertedMessages := make([]*bridgev2.BackfillMessage, 0, len(chat.ChatItems))
	for i := range chat.ChatItems {
		item := &chat.ChatItems[i]
		msgID := simplexid.MakeMessageID(item.Meta.ItemID)
		ts := parseSimplexTime(item.Meta.CreatedAt)
		sender := s.makeEventSenderFromDir(item.ChatDir)
		if item.ChatDir.Type == "directRcv" && chat.ChatInfo.Contact != nil {
			sender = s.makeEventSenderFromContact(chat.ChatInfo.Contact)
		}

		cm := convertChatItemToMatrix(item)

		var reactions []*bridgev2.BackfillReaction
		for _, reaction := range item.Reactions {
			if reaction.Reaction.Type != "emoji" {
				continue
			}
			// We don't have per-reactor data in CIReactionCount, skip individuals.
			_ = reaction
		}

		convertedMessages = append(convertedMessages, &bridgev2.BackfillMessage{
			ConvertedMessage: cm,
			Sender:           sender,
			ID:               msgID,
			TxnID:            networkid.TransactionID(msgID),
			Timestamp:        ts,
			StreamOrder:      ts.UnixMilli(),
			Reactions:        reactions,
		})
	}

	hasMore := len(chat.ChatItems) >= params.Count
	return &bridgev2.FetchMessagesResponse{
		Messages:         convertedMessages,
		HasMore:          hasMore,
		Forward:          false,
		MarkRead:         false,
		ApproxTotalCount: 0,
		CompleteCallback: func() {
			zerolog.Ctx(ctx).Debug().
				Int("count", len(convertedMessages)).
				Time("oldest", func() time.Time {
					if len(convertedMessages) > 0 {
						return convertedMessages[0].Timestamp
					}
					return time.Time{}
				}()).
				Msg("Backfill batch complete")
		},
	}, nil
}
