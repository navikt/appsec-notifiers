package slack

import (
	"context"
	"fmt"
	"time"

	"github.com/slack-go/slack"
)

const historyPageSize = 200

// ConversationInfo returns the channel as the bot sees it.
func (c *Client) ConversationInfo(ctx context.Context, channelID string) (*slack.Channel, error) {
	return c.api.GetConversationInfoContext(ctx, &slack.GetConversationInfoInput{ChannelID: channelID})
}

// FindMessageByMetadata searches channel history newer than oldest for a message whose metadata has
// the given event type and a string payload field key equal to value. It reads at most maxPages pages
// and returns found=false if no match is found within them.
func (c *Client) FindMessageByMetadata(ctx context.Context, channelID string, oldest time.Time, eventType, key, value string, maxPages int) (ts string, found bool, err error) {
	params := &slack.GetConversationHistoryParameters{
		ChannelID:          channelID,
		Oldest:             fmt.Sprintf("%d.000000", oldest.Unix()),
		Limit:              historyPageSize,
		IncludeAllMetadata: true,
	}
	for range maxPages {
		resp, err := c.api.GetConversationHistoryContext(ctx, params)
		if err != nil {
			return "", false, err
		}
		for _, m := range resp.Messages {
			if m.Metadata.EventType != eventType {
				continue
			}
			if v, ok := m.Metadata.EventPayload[key].(string); ok && v == value {
				return m.Timestamp, true, nil
			}
		}
		if !resp.HasMore || resp.ResponseMetaData.NextCursor == "" {
			return "", false, nil
		}
		params.Cursor = resp.ResponseMetaData.NextCursor
	}
	c.log.WithField("channel_id", channelID).Warnf("history search stopped after %d pages without reaching the oldest message", maxPages)
	return "", false, nil
}

// PostBlocks posts a Block Kit message with plain fallback text and optional metadata, and returns
// the message timestamp. fallbackText must already be escaped for Slack mrkdwn.
func (c *Client) PostBlocks(ctx context.Context, channelID, fallbackText string, blocks []slack.Block, metadata *slack.SlackMetadata) (string, error) {
	opts := []slack.MsgOption{
		slack.MsgOptionText(fallbackText, false),
		slack.MsgOptionBlocks(blocks...),
		slack.MsgOptionDisableLinkUnfurl(),
		slack.MsgOptionDisableMediaUnfurl(),
	}
	if metadata != nil {
		opts = append(opts, slack.MsgOptionMetadata(*metadata))
	}
	_, ts, err := c.api.PostMessageContext(ctx, channelID, opts...)
	return ts, err
}
