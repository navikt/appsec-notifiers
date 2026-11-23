package deliveryapi

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"
)

func testDigest(items, total int) *digestRequest {
	d := &digestRequest{
		Team:       "appsec",
		Period:     &period{From: time.Date(2026, 11, 16, 6, 0, 0, 0, time.UTC), To: time.Date(2026, 11, 23, 6, 0, 0, 0, time.UTC)},
		TptURL:     "https://tpt.ansatt.nav.no/team/appsec",
		TotalCount: &total,
	}
	score := 90.0
	for i := range items {
		d.Items = append(d.Items, digestItem{
			Workload: "app", Environment: "prod-gcp", CveID: fmt.Sprintf("CVE-2026-%04d", i),
			RiskScore: &score, Band: bandImmediately, Reason: reasonNew, URL: "https://tpt.ansatt.nav.no/v",
		})
	}
	return d
}

func blocksJSON(t *testing.T, msg renderedMessage) string {
	t.Helper()
	var b strings.Builder
	enc := json.NewEncoder(&b)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(msg.blocks); err != nil {
		t.Fatal(err)
	}
	return b.String()
}

func TestRenderDigest_TruncatesToSlackBlockLimit(t *testing.T) {
	msg := renderDigest(testDigest(120, 150))

	if len(msg.blocks) > 50 {
		t.Fatalf("blocks = %d, Slack allows at most 50", len(msg.blocks))
	}
	raw := blocksJSON(t, msg)
	if !strings.Contains(raw, "…og 110 til. <https://tpt.ansatt.nav.no/team/appsec|") {
		t.Errorf("expected 'and N more' line linking to TPT, got %s", raw)
	}
	if strings.Contains(raw, fmt.Sprintf("CVE-2026-%04d", maxItemsShown)) {
		t.Errorf("rendered more than %d items", maxItemsShown)
	}
}

func TestRenderDigest_FormatsInNorwegianAndEscapesInput(t *testing.T) {
	d := testDigest(1, 1)
	pkg := "lib<script>&`x`"
	score := 7.3
	d.Items[0].PackageName = &pkg
	d.Items[0].RiskScore = &score
	d.Items[0].Band = bandPrioritize
	d.Items[0].Reason = reasonReappeared

	msg := renderDigest(d)

	got := blocksJSON(t, msg)
	for _, want := range []string{
		"1 ny sårbarhet med høy risiko fra 16.11.2026 kl. 07.00 til 23.11.2026 kl. 07.00.",
		"lib&lt;script&gt;&amp;'x'",
		"Må prioriteres · risikoscore 7,3 · Dukket opp igjen",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("blocks missing %q:\n%s", want, got)
		}
	}
	if strings.Contains(got, "og 0 til") {
		t.Errorf("should not show 'and N more' when everything fits")
	}
}
