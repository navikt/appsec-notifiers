package deliveryapi

import (
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"
	_ "time/tzdata"

	"github.com/slack-go/slack"
)

const (
	// Slack allows 50 blocks per message. A digest uses at most five blocks besides its items.
	maxItemsShown  = 40
	maxFieldRunes  = 150
	maxHeaderRunes = 150
)

var oslo = mustLoadLocation("Europe/Oslo")

func mustLoadLocation(name string) *time.Location {
	loc, err := time.LoadLocation(name)
	if err != nil {
		panic(err)
	}
	return loc
}

type renderedMessage struct {
	fallback string
	blocks   []slack.Block
}

var bandLabels = map[string]string{
	bandImmediately: "Snarest",
	bandPrioritize:  "Må prioriteres",
}

var reasonLabels = map[string]string{
	reasonNew:        "Ny",
	reasonReappeared: "Dukket opp igjen",
	reasonEscalated:  "Flyttet til Snarest",
}

func renderDigest(d *digestRequest) renderedMessage {
	total := *d.TotalCount
	shown := d.Items[:min(len(d.Items), maxItemsShown)]
	remaining := total - len(shown)

	summary := fmt.Sprintf("%s med høy risiko fra %s til %s.",
		countPhrase(total), formatTime(d.Period.From), formatTime(d.Period.To))

	blocks := []slack.Block{
		headerBlock("Nye sårbarheter for " + d.Team),
		mrkdwnSection(summary),
		slack.NewDividerBlock(),
	}
	for _, item := range shown {
		blocks = append(blocks, mrkdwnSection(renderItem(item)))
	}
	if len(shown) > 0 {
		blocks = append(blocks, slack.NewDividerBlock())
	}
	link := fmt.Sprintf("<%s|Se alle sårbarhetene til %s i TPT>", d.TptURL, escapeMrkdwn(d.Team))
	if remaining > 0 {
		blocks = append(blocks, mrkdwnSection(fmt.Sprintf("…og %d til. %s", remaining, link)))
	} else {
		blocks = append(blocks, mrkdwnSection(link))
	}

	fallback := fmt.Sprintf("%s med høy risiko for teamet %s. Se alle i TPT: <%s>",
		countPhrase(total), escapeMrkdwn(d.Team), d.TptURL)
	return renderedMessage{fallback: fallback, blocks: blocks}
}

func renderItem(it digestItem) string {
	var b strings.Builder
	fmt.Fprintf(&b, "*<%s|%s>*", it.URL, escapeMrkdwn(truncate(it.CveID, maxFieldRunes)))
	if it.PackageName != nil && strings.TrimSpace(*it.PackageName) != "" {
		fmt.Fprintf(&b, " i %s", codeSpan(*it.PackageName))
	}
	fmt.Fprintf(&b, "\n%s i %s", codeSpan(it.Workload), escapeMrkdwn(truncate(it.Environment, maxFieldRunes)))
	fmt.Fprintf(&b, "\n%s · risikoscore %s · %s", bandLabels[it.Band], formatScore(*it.RiskScore), reasonLabels[it.Reason])
	return b.String()
}

func renderTestMessage(team string) renderedMessage {
	text := fmt.Sprintf("Dette er en testmelding fra TPT for teamet %s. Kanalen er klar til å ta imot varsler om nye sårbarheter med høy risiko.", escapeMrkdwn(team))
	return renderedMessage{
		fallback: text,
		blocks:   []slack.Block{headerBlock("Testmelding fra TPT"), mrkdwnSection(text)},
	}
}

func headerBlock(text string) slack.Block {
	return slack.NewHeaderBlock(slack.NewTextBlockObject(slack.PlainTextType, truncate(text, maxHeaderRunes), false, false))
}

func mrkdwnSection(text string) slack.Block {
	return slack.NewSectionBlock(slack.NewTextBlockObject(slack.MarkdownType, text, false, false), nil, nil)
}

func countPhrase(n int) string {
	if n == 1 {
		return "1 ny sårbarhet"
	}
	return fmt.Sprintf("%d nye sårbarheter", n)
}

func formatTime(t time.Time) string {
	return t.In(oslo).Format("02.01.2006 kl. 15.04")
}

func formatScore(score float64) string {
	if score == math.Trunc(score) {
		return strconv.FormatFloat(score, 'f', 0, 64)
	}
	return strings.Replace(strconv.FormatFloat(score, 'f', 1, 64), ".", ",", 1)
}

func codeSpan(s string) string {
	return "`" + escapeMrkdwn(truncate(strings.ReplaceAll(s, "`", "'"), maxFieldRunes)) + "`"
}

// escapeMrkdwn escapes the control characters Slack requires in mrkdwn text.
func escapeMrkdwn(s string) string {
	return strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;").Replace(s)
}

func truncate(s string, maxRunes int) string {
	r := []rune(s)
	if len(r) <= maxRunes {
		return s
	}
	return string(r[:maxRunes-1]) + "…"
}
