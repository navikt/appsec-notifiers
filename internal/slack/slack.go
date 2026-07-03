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

func (c *Client) SendCustomMessageToChannel(ctx context.Context, channelName, messageText string) error {
	sectionBlock := slack.NewSectionBlock(
		slack.NewTextBlockObject(slack.MarkdownType, messageText, false, false),
		nil,
		nil,
	)

	_, _, err := c.api.PostMessageContext(
		ctx,
		channelName,
		slack.MsgOptionBlocks(sectionBlock),
	)
	if err != nil {
		return fmt.Errorf("post custom message to channel %s: %w", channelName, err)
	}

	return nil
}

func (c *Client) SendSecretScanningAlert(ctx context.Context, channelName, repoFullName, repoName, secretType string) error {
	heading := fmt.Sprintf(`:wave: Hei, eier av %s :github2:`, repoName)
	repoLink := fmt.Sprintf(`<https://github.com/%s/security/secret-scanning|%s (%s)>`, repoFullName, repoName, secretType)
	text := fmt.Sprintf(
		"GitHub har oppdaget hemmeligheter i repo som dere eier:\n\n %s\n\n Dersom hemmelighetene er aktive må de *roteres* så fort som mulig, og videre varsling og steg for å avdekke evt. misbruk må iverksettes. \n\n :warning: Husk at Git aldri glemmer, så kun fjerning fra koden er IKKE tilstrekkelig.\n\nNår dette er gjort (eller dersom dette er falske positiver) lukkes varselet ved å velge i nedtrekksmenyen `Close as`.\n\nDu kan også lese mer om håndtering av hemmeligheter i vår <https://sikkerhet.nav.no/docs/sikker-utvikling/hemmeligheter|Security Playbook>\nDenne kanalen ble benyttet fordi dere har oppgitt den som ønsket varslingskanal i Console eller Teamkatalogen.",
		repoLink,
	)

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
		return fmt.Errorf("post secret scanning alert to channel %s: %w", channelName, err)
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
