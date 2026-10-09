package memory

import (
	"strings"
	"testing"
)

func TestParseObservations(t *testing.T) {
	out := `<observation>
  <type>bugfix</type>
  <title>Fixed auth token refresh</title>
  <subtitle>401 on expired refresh tokens</subtitle>
  <facts>
    <fact>internal/auth/token.go: 401 on expired tokens</fact>
    <fact>Added retry-once</fact>
  </facts>
  <concepts>
    <concept>auth</concept>
    <concept>tokens</concept>
  </concepts>
  <narrative>The refresh endpoint rejected expired tokens.</narrative>
</observation>
<observation>
  <type>decision</type>
  <title>Chose sqlite over postgres</title>
  <narrative>Kept local sqlite for zero-ops.</narrative>
</observation>`

	obs, ok := parseObservations(out)
	if !ok {
		t.Fatal("parse failed")
	}
	if len(obs) != 2 {
		t.Fatalf("got %d observations, want 2", len(obs))
	}
	first := obs[0]
	if first.Type != "bugfix" || first.Title != "Fixed auth token refresh" {
		t.Fatalf("first = %+v", first)
	}
	if len(first.Facts) != 2 || first.Facts[1] != "Added retry-once" {
		t.Fatalf("facts = %v", first.Facts)
	}
	if len(first.Concepts) != 2 || first.Concepts[0] != "auth" {
		t.Fatalf("concepts = %v", first.Concepts)
	}
}

func TestParseObservationsSkip(t *testing.T) {
	out := `<skip_summary reason="noise"/>`
	obs, ok := parseObservations(out)
	if !ok || len(obs) != 0 {
		t.Fatalf("expected skip (ok=true, no observations), got %v ok=%v", obs, ok)
	}
}

func TestParseObservationsSalvage(t *testing.T) {
	out := `<observation>
  <type>bugfix</type>
  <title>   </title>
  <narrative>Fixed the flaky test timeout.
The timeout was too short under CI load.</narrative>
</observation>`
	obs, ok := parseObservations(out)
	if !ok || len(obs) != 1 {
		t.Fatalf("got %v ok=%v", obs, ok)
	}
	if obs[0].Title != "Fixed the flaky test timeout." {
		t.Fatalf("salvaged title = %q", obs[0].Title)
	}
	if strings.Contains(obs[0].Narrative, "Fixed the flaky") {
		t.Fatalf("narrative should not repeat the title: %q", obs[0].Narrative)
	}
}

func TestParseObservationsInvalidType(t *testing.T) {
	out := `<observation><type>unknown</type><title>T</title><narrative>N</narrative></observation>`
	obs, ok := parseObservations(out)
	if !ok {
		t.Fatal("parse failed")
	}
	if obs[0].Type != "change" {
		t.Fatalf("type = %q, want change", obs[0].Type)
	}
}

func TestParseSummary(t *testing.T) {
	out := `<summary>
  <request>Fix the auth flow</request>
  <learned>Refresh tokens expire after 24h</learned>
  <completed>Fixed the refresh endpoint</completed>
  <next_steps>Add tests for expiry edge cases</next_steps>
</summary>`
	sum, ok := parseSummary(out)
	if !ok {
		t.Fatal("parse failed")
	}
	if sum.Request != "Fix the auth flow" || sum.Learned != "Refresh tokens expire after 24h" {
		t.Fatalf("sum = %+v", sum)
	}
	if sum.NextSteps != "Add tests for expiry edge cases" {
		t.Fatalf("next steps = %q", sum.NextSteps)
	}
}

func TestParseSummarySkip(t *testing.T) {
	out := `<skip_summary reason="nothing durable"/>`
	if _, ok := parseSummary(out); ok {
		t.Fatal("expected skip")
	}
}

func TestObservationBlockRoundtrip(t *testing.T) {
	obs := Observation{
		ID: "obs-9f3a1b", Type: "bugfix", Title: "Fixed auth",
		Subtitle: "401 on refresh", Facts: []string{"f1", "f2"},
		Concepts: []string{"auth", "token"}, Narrative: "The endpoint rejected expired tokens.",
		Hash: "9f3a1b", At: timeNow(),
	}
	block := observationBlock(obs)
	if !strings.HasPrefix(block, "## obs-9f3a1b [bugfix] Fixed auth") {
		t.Fatalf("block header wrong: %q", block)
	}
	if !strings.Contains(block, "- facts:") || !strings.Contains(block, "f1") {
		t.Fatalf("facts missing: %q", block)
	}
	if !strings.Contains(block, "- keywords: auth, token") {
		t.Fatalf("keywords missing: %q", block)
	}
	if !strings.Contains(block, "- hash: 9f3a1b") {
		t.Fatalf("hash missing: %q", block)
	}
}

func TestFileEvidence(t *testing.T) {
	read, mod := fileEvidence("read", `{"path":"internal/auth/token.go"}`)
	if len(read) != 1 || read[0] != "internal/auth/token.go" {
		t.Fatalf("read = %v", read)
	}
	if len(mod) != 0 {
		t.Fatalf("mod = %v", mod)
	}
	read, mod = fileEvidence("edit", `{"file_paths":["a.go","b.go"]}`)
	if len(mod) != 2 {
		t.Fatalf("mod = %v", mod)
	}
	read, mod = fileEvidence("bash", `{"command":"go test ./..."}`)
	if len(read) != 0 || len(mod) != 0 {
		t.Fatalf("bash evidence = %v/%v", read, mod)
	}
}

func TestBuildObservationInput(t *testing.T) {
	input, err := buildObservationInput([]observedUse{
		{ToolName: "bash", Params: `{"command":"ls"}`, Outcome: "ok", At: timeNow()},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(input, "<observed_from_primary_session>") ||
		!strings.Contains(input, "bash") {
		t.Fatalf("input = %q", input)
	}
}
