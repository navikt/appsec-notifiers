package slack

import (
	"context"
	"fmt"

	"github.com/sirupsen/logrus"
	"github.com/slack-go/slack"
)

type Client struct {
	api *slack.Client
	log logrus.FieldLogger
}

func NewClient(apiToken string, log logrus.FieldLogger) *Client {
	return &Client{
		api: slack.New(apiToken),
		log: log,
	}
}

func (c *Client) SendMessage(ctx context.Context, channelName, teamSlug, repoName string) error {
	heading := fmt.Sprintf(`:wave: Hei, %s :github2:`, teamSlug)
	text := fmt.Sprintf(`Dere har knyttet GitHub-repoet <https://github.com/%[1]s|%[1]s> opp til teamet deres via <https://console.nav.cloud.nais.io/team/%[2]s/repositories|Console>. Dette repoet har ikke Dependabot alerts aktivert. Dependabot hjelper deg å oppdage biblioteker med kjente sårbarheter i appene dine. Du kan sjekke status og enable Dependabot <https://github.com/%[1]s/security|her>. Hvis repoet ikke er i bruk, vurder å arkivere det. Det kan gjøres nederst på <https://github.com/%[1]s/settings|denne siden>.`, repoName, teamSlug)

	headerBlock := slack.NewHeaderBlock(
		slack.NewTextBlockObject(slack.PlainTextType, heading, false, false),
	)
	dividerBlock := slack.NewDividerBlock()
	sectionBlock := slack.NewSectionBlock(
		slack.NewTextBlockObject(slack.MarkdownType, text, false, false),
		nil,
		nil,
	)

	_, _, err := c.api.PostMessageContext(
		ctx,
		channelName,
		slack.MsgOptionBlocks(headerBlock, dividerBlock, sectionBlock),
	)
	if err != nil {
		return fmt.Errorf("post message to channel %s: %w", channelName, err)
	}

	return nil
}

// FindUserByEmail looks up a Slack user ID by their email address
func (c *Client) FindUserByEmail(ctx context.Context, email string) (string, error) {
	user, err := c.api.GetUserByEmailContext(ctx, email)
	if err != nil {
		return "", fmt.Errorf("lookup user by email %s: %w", email, err)
	}
	return user.ID, nil
}

// SendDirectMessages sends a direct message to each user ID in the list
func (c *Client) SendDirectMessages(ctx context.Context, userIDs []string, messageText string) error {
	for _, userID := range userIDs {
		if err := c.sendDirectMessage(ctx, userID, messageText); err != nil {
			c.log.WithError(err).WithField("user_id", userID).Errorf("failed to send direct message")
			// Continue to try sending to other users even if one fails
			continue
		}
		c.log.WithField("user_id", userID).Infof("successfully sent direct message")
	}
	return nil
}

func (c *Client) sendDirectMessage(ctx context.Context, userID, messageText string) error {
	sectionBlock := slack.NewSectionBlock(
		slack.NewTextBlockObject(slack.MarkdownType, messageText, false, false),
		nil,
		nil,
	)

	_, _, err := c.api.PostMessageContext(
		ctx,
		userID,
		slack.MsgOptionBlocks(sectionBlock),
	)
	if err != nil {
		return fmt.Errorf("post direct message to user %s: %w", userID, err)
	}

	return nil
}
