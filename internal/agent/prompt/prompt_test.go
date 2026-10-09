package prompt

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSkillsList(t *testing.T) {
	items := []SkillInfo{
		{Name: "go-development", Description: "Go conventions"},
		{Name: "pdf-tools", Description: "PDF <extraction>"},
	}

	md, err := New().Add(SkillsList(items)).Build(FormatMarkdown)
	if err != nil {
		t.Fatal(err)
	}
	wantMD := "- **go-development**: Go conventions\n- **pdf-tools**: PDF <extraction>"
	if md != wantMD {
		t.Fatalf("markdown:\ngot:\n%s\nwant:\n%s", md, wantMD)
	}

	xm, err := New().Add(SkillsList(items)).Build(FormatXML)
	if err != nil {
		t.Fatal(err)
	}
	wantXML := "<skills>\n" +
		"  <skill name=\"go-development\" description=\"Go conventions\"/>\n" +
		"  <skill name=\"pdf-tools\" description=\"PDF &lt;extraction&gt;\"/>\n" +
		"</skills>"
	if xm != wantXML {
		t.Fatalf("xml:\ngot:\n%s\nwant:\n%s", xm, wantXML)
	}

	js, err := New().Add(SkillsList(items)).Build(FormatJSON)
	if err != nil {
		t.Fatal(err)
	}
	wantJSON := "[\n" +
		"  {\n" +
		"    \"items\": [\n" +
		"      {\n        \"description\": \"Go conventions\",\n        \"name\": \"go-development\"\n      },\n" +
		"      {\n        \"description\": \"PDF <extraction>\",\n        \"name\": \"pdf-tools\"\n      }\n" +
		"    ],\n" +
		"    \"type\": \"skills\"\n" +
		"  }\n" +
		"]"
	if js != wantJSON {
		t.Fatalf("json:\ngot:\n%s\nwant:\n%s", js, wantJSON)
	}

	empty, err := New().Add(SkillsList(nil)).Build(FormatMarkdown)
	if err != nil {
		t.Fatal(err)
	}
	if empty != "" {
		t.Fatalf("empty list must render nothing, got %q", empty)
	}

	chain, err := New().Skills(items).Build(FormatMarkdown)
	if err != nil {
		t.Fatal(err)
	}
	if chain != wantMD {
		t.Fatalf("chain Skills() = %q, want %q", chain, wantMD)
	}
}

func TestBuildEmpty(t *testing.T) {
	p := New()
	for _, tc := range []struct {
		format Format
		want   string
	}{
		{FormatMarkdown, ""},
		{FormatXML, ""},
		{FormatJSON, "[]"},
	} {
		got, err := p.Build(tc.format)
		if err != nil {
			t.Fatalf("Build(%v): %v", tc.format, err)
		}
		if got != tc.want {
			t.Fatalf("Build(%v)=%q want %q", tc.format, got, tc.want)
		}
	}
}

func TestMarkdownGolden(t *testing.T) {
	p := New().
		Set("name", "Omer").
		Title("System Prompt").
		Heading(2, "Role").
		Paragraph("You are {{name}}, a helpful assistant.").
		List("be concise", "be accurate").
		OrderedList("first", "second").
		Code("go", "fmt.Println(\"hi\")").
		KV("model", "deepseek-v4").
		Table([]string{"a", "b"}, [][]string{{"1", "2"}, {"3", "4"}}).
		Comment("generated")

	want := "# System Prompt\n\n" +
		"## Role\n\n" +
		"You are Omer, a helpful assistant.\n\n" +
		"- be concise\n- be accurate\n\n" +
		"1. first\n2. second\n\n" +
		"```go\nfmt.Println(\"hi\")\n```\n\n" +
		"**model**: deepseek-v4\n\n" +
		"| a | b |\n" +
		"| --- | --- |\n" +
		"| 1 | 2 |\n" +
		"| 3 | 4 |\n\n" +
		"<!-- generated -->"

	got, err := p.Build(FormatMarkdown)
	if err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Fatalf("markdown:\ngot:\n%q\nwant:\n%q", got, want)
	}
}

func TestXMLGolden(t *testing.T) {
	p := New().
		Set("name", "Omer").
		Title("System Prompt").
		Heading(2, "Role").
		Paragraph("You are {{name}}, a helpful assistant.").
		List("be concise", "be accurate").
		OrderedList("first", "second").
		Code("go", "fmt.Println(\"hi\")").
		KV("model", "deepseek-v4").
		Table([]string{"a", "b"}, [][]string{{"1", "2"}, {"3", "4"}}).
		Comment("generated")

	want := "<title>System Prompt</title>\n" +
		"<heading level=\"2\">Role</heading>\n" +
		"<paragraph>You are Omer, a helpful assistant.</paragraph>\n" +
		"<list ordered=\"false\">\n  <item>be concise</item>\n  <item>be accurate</item>\n</list>\n" +
		"<list ordered=\"true\">\n  <item>first</item>\n  <item>second</item>\n</list>\n" +
		"<code lang=\"go\">fmt.Println(&quot;hi&quot;)</code>\n" +
		"<kv key=\"model\">deepseek-v4</kv>\n" +
		"<table>\n  <header><cell>a</cell><cell>b</cell></header>\n" +
		"  <row><cell>1</cell><cell>2</cell></row>\n" +
		"  <row><cell>3</cell><cell>4</cell></row>\n</table>\n" +
		"<!-- generated -->"

	got, err := p.Build(FormatXML)
	if err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Fatalf("xml:\ngot:\n%q\nwant:\n%q", got, want)
	}
}

func TestJSONGolden(t *testing.T) {
	p := New().Set("n", 7).Title("T").Paragraph("v={{n}}").List("a", "b").KV("k", "v")
	want := "[\n" +
		"  {\n" +
		"    \"text\": \"T\",\n" +
		"    \"type\": \"title\"\n" +
		"  },\n" +
		"  {\n" +
		"    \"text\": \"v=7\",\n" +
		"    \"type\": \"paragraph\"\n" +
		"  },\n" +
		"  {\n" +
		"    \"items\": [\n" +
		"      \"a\",\n" +
		"      \"b\"\n" +
		"    ],\n" +
		"    \"ordered\": false,\n" +
		"    \"type\": \"list\"\n" +
		"  },\n" +
		"  {\n" +
		"    \"key\": \"k\",\n" +
		"    \"type\": \"kv\",\n" +
		"    \"value\": \"v\"\n" +
		"  }\n" +
		"]"
	got, err := p.Build(FormatJSON)
	if err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Fatalf("json:\ngot:\n%s\nwant:\n%s", got, want)
	}
}

func TestMixedPerPartFormats(t *testing.T) {
	p := New().
		Title("Doc").
		Add(WithFormat(FormatXML, List("a", "b"))).
		Add(WithFormat(FormatJSON, KV("k", "v")))

	md, err := p.Build(FormatMarkdown)
	if err != nil {
		t.Fatal(err)
	}
	wantMD := "# Doc\n\n" +
		"<list ordered=\"false\">\n  <item>a</item>\n  <item>b</item>\n</list>\n\n" +
		"{\"key\":\"k\",\"type\":\"kv\",\"value\":\"v\"}"
	if md != wantMD {
		t.Fatalf("mixed markdown:\ngot:\n%s\nwant:\n%s", md, wantMD)
	}

	js, err := p.Build(FormatJSON)
	if err != nil {
		t.Fatal(err)
	}
	wantJS := "[\n" +
		"  {\n    \"text\": \"Doc\",\n    \"type\": \"title\"\n  },\n" +
		"  {\n    \"format\": \"xml\",\n    \"rendered\": \"<list ordered=\\\"false\\\">\\n  <item>a</item>\\n  <item>b</item>\\n</list>\"\n  },\n" +
		"  {\n    \"format\": \"json\",\n    \"rendered\": \"{\\\"key\\\":\\\"k\\\",\\\"type\\\":\\\"kv\\\",\\\"value\\\":\\\"v\\\"}\"\n  }\n" +
		"]"
	if js != wantJS {
		t.Fatalf("mixed json:\ngot:\n%s\nwant:\n%s", js, wantJS)
	}
}

func TestIfConditions(t *testing.T) {
	p := New().Set("mode", "strict").
		Add(If("mode == \"strict\"", Paragraph("A"))).
		Add(If("mode == \"loose\"", Paragraph("C"))).
		Add(If("mode == \"loose\"", Paragraph("D")).Else(Paragraph("E"))).
		Add(If("mode == \"strict\"", Paragraph("F")).
			ElseIf("mode == \"loose\"", Paragraph("G")).
			Else(Paragraph("H"))).
		Add(If("mode == \"loose\"", Paragraph("I")).
			ElseIf("mode == \"strict\"", Paragraph("J")).
			Else(Paragraph("K"))).
		Add(IfFn(func(v Vars) bool { return v.Bool("verbose") }, Paragraph("L")).Else(Paragraph("M")))

	got, err := p.Build(FormatMarkdown)
	if err != nil {
		t.Fatal(err)
	}
	want := "A\n\nE\n\nF\n\nJ\n\nM"
	if got != want {
		t.Fatalf("got:\n%q\nwant:\n%q", got, want)
	}
}

func TestChainIf(t *testing.T) {
	p := New().Set("x", 1).
		If("x == 1",
			func(q *Prompt) { q.Paragraph("one") },
			func(q *Prompt) { q.Paragraph("two") }).
		When("x == 1", func(q *Prompt) { q.Paragraph("when") }).
		IfFn(func(Vars) bool { return true }, func(q *Prompt) { q.Paragraph("fn") })

	got, err := p.Build(FormatMarkdown)
	if err != nil {
		t.Fatal(err)
	}
	want := "one\n\nwhen\n\nfn"
	if got != want {
		t.Fatalf("got:\n%q\nwant:\n%q", got, want)
	}
}

func TestNestedConditionals(t *testing.T) {
	p := New().Set("a", true).Set("b", true).
		Add(If("a == true", If("b == true", Paragraph("both"))))
	got, err := p.Build(FormatMarkdown)
	if err != nil {
		t.Fatal(err)
	}
	if got != "both" {
		t.Fatalf("got %q want %q", got, "both")
	}
}

func TestSwitch(t *testing.T) {
	build := func(vars Vars) string {
		p := New().SetVars(vars).Add(
			Switch("role").
				Case("\"admin\"", Paragraph("Admin instructions")).
				Case("\"user\"", Paragraph("User instructions")).
				Default(Paragraph("Guest instructions")),
		)
		out, err := p.Build(FormatMarkdown)
		if err != nil {
			t.Fatal(err)
		}
		return out
	}
	if got := build(Vars{"role": "admin"}); got != "Admin instructions" {
		t.Fatalf("admin: %q", got)
	}
	if got := build(Vars{"role": "user"}); got != "User instructions" {
		t.Fatalf("user: %q", got)
	}
	if got := build(Vars{"role": "guest"}); got != "Guest instructions" {
		t.Fatalf("guest: %q", got)
	}

	p := New().Set("count", 5).Add(
		Switch("count").
			Case("count >= 3", Paragraph("many")).
			Default(Paragraph("few")),
	)
	got, err := p.Build(FormatMarkdown)
	if err != nil {
		t.Fatal(err)
	}
	if got != "many" {
		t.Fatalf("comparison case: %q", got)
	}
}

func TestChainSwitch(t *testing.T) {
	p := New().Set("role", "admin").
		Switch("role", func(s *SwitchPart) {
			s.Case("\"admin\"", Paragraph("A")).Default(Paragraph("B"))
		})
	got, err := p.Build(FormatMarkdown)
	if err != nil {
		t.Fatal(err)
	}
	if got != "A" {
		t.Fatalf("got %q want %q", got, "A")
	}
}

func TestXMLTag(t *testing.T) {
	p := New().XMLTag("system", Title("Sys"), Paragraph("hi"))
	want := "<system>\n  <title>Sys</title>\n  <paragraph>hi</paragraph>\n</system>"
	for _, format := range []Format{FormatMarkdown, FormatXML} {
		got, err := p.Build(format)
		if err != nil {
			t.Fatal(err)
		}
		if got != want {
			t.Fatalf("%v:\ngot:\n%s\nwant:\n%s", format, got, want)
		}
	}
}

func TestEscapeXML(t *testing.T) {
	p := New().Paragraph(`a < b & c > d "e"`)
	got, err := p.Build(FormatXML)
	if err != nil {
		t.Fatal(err)
	}
	want := `<paragraph>a &lt; b &amp; c &gt; d &quot;e&quot;</paragraph>`
	if got != want {
		t.Fatalf("got %q want %q", got, want)
	}
}

func TestInterpolation(t *testing.T) {
	got, err := Render("Hi {{name}}, you have {{stats.count}} items", Vars{
		"name":  "Omer",
		"stats": map[string]any{"count": 3},
	})
	if err != nil {
		t.Fatal(err)
	}
	if got != "Hi Omer, you have 3 items" {
		t.Fatalf("got %q", got)
	}
}

func TestMissingVariableError(t *testing.T) {
	if _, err := Render("Hi {{missing}}", Vars{"name": "Omer"}); err == nil {
		t.Fatal("expected error for missing variable")
	}
	p := New().Paragraph("Hi {{missing}}")
	if _, err := p.Build(FormatMarkdown); err == nil {
		t.Fatal("expected Build error for missing variable")
	}
}

func TestExpressionErrors(t *testing.T) {
	cases := []*Prompt{
		New().Add(If("mode >=", Paragraph("x"))),
		New().Set("n", 1).Add(If("n > \"x\"", Paragraph("y"))),
		New().Add(If("missing_var", Paragraph("x"))),
		New().Set("n", 1).Add(If("1 + 1", Paragraph("x"))),
		New().Add(If("(", Paragraph("x"))),
		New().Add(Switch("role").Case("count >", Paragraph("x"))),
	}
	for i, p := range cases {
		if _, err := p.Build(FormatMarkdown); err == nil {
			t.Fatalf("case %d: expected error", i)
		}
	}
}

func TestSelectAndIDs(t *testing.T) {
	p := New().
		Title("T").
		Add(WithID("tools", List("a"))).
		Add(WithID("notes", Paragraph("n")))

	ids := p.IDs()
	if len(ids) != 2 || ids[0] != "tools" || ids[1] != "notes" {
		t.Fatalf("IDs() = %v", ids)
	}

	sel := p.Select("tools")
	got, err := sel.Build(FormatMarkdown)
	if err != nil {
		t.Fatal(err)
	}
	if got != "- a" {
		t.Fatalf("Select(tools) = %q", got)
	}

	got, err = p.Select("missing").Build(FormatMarkdown)
	if err != nil {
		t.Fatal(err)
	}
	if got != "" {
		t.Fatalf("Select(missing) = %q", got)
	}
}

func TestGroup(t *testing.T) {
	p := New().
		Title("T").
		Add(WithID("block", Group(Heading(1, "A"), Paragraph("b"))))

	got, err := p.Build(FormatMarkdown)
	if err != nil {
		t.Fatal(err)
	}
	if got != "# T\n\n# A\n\nb" {
		t.Fatalf("markdown: %q", got)
	}

	sel, err := p.Select("block").Build(FormatMarkdown)
	if err != nil {
		t.Fatal(err)
	}
	if sel != "# A\n\nb" {
		t.Fatalf("select: %q", sel)
	}

	js, err := p.Build(FormatJSON)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(js, `"type": "group"`) {
		t.Fatalf("json lacks group node: %s", js)
	}
}

func TestClone(t *testing.T) {
	p := New().Set("a", "1").Paragraph("{{a}}")
	q := p.Clone()
	q.Set("a", "2").Add(Paragraph("extra"))

	a, _ := p.Build(FormatMarkdown)
	b, _ := q.Build(FormatMarkdown)
	if a != "1" {
		t.Fatalf("original mutated: %q", a)
	}
	if b != "2\n\nextra" {
		t.Fatalf("clone = %q", b)
	}
}

func TestMustBuildPanics(t *testing.T) {
	p := New().Add(If("bad syntax", Paragraph("x")))
	defer func() {
		if recover() == nil {
			t.Fatal("expected panic")
		}
	}()
	_ = p.MustBuild(FormatMarkdown)
}

func TestUnsupportedFormat(t *testing.T) {
	p := New().Title("x")
	if _, err := p.Build(Format(99)); err == nil {
		t.Fatal("expected error for unsupported format")
	}
}

func TestWithFormatInvalidPanics(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Fatal("expected panic")
		}
	}()
	WithFormat(Format(99), Paragraph("x"))
}

func TestWithIDEmptyNoop(t *testing.T) {
	p := New().Add(WithID("", Paragraph("x")))
	if got, err := p.Build(FormatMarkdown); err != nil || got != "x" {
		t.Fatalf("got=%q err=%v", got, err)
	}
}

func TestJSONIfSwitchNodes(t *testing.T) {
	p := New().Set("mode", "strict").Set("role", "admin").
		Add(If("mode == \"strict\"", Paragraph("A")).Else(Paragraph("B"))).
		Add(Switch("role").Case("\"admin\"", Paragraph("C")))

	got, err := p.Build(FormatJSON)
	if err != nil {
		t.Fatal(err)
	}
	want := "[\n" +
		"  {\n" +
		"    \"condition\": \"mode == \\\"strict\\\"\",\n" +
		"    \"parts\": [\n" +
		"      {\n        \"text\": \"A\",\n        \"type\": \"paragraph\"\n      }\n" +
		"    ],\n" +
		"    \"taken\": \"then\",\n" +
		"    \"type\": \"if\"\n" +
		"  },\n" +
		"  {\n" +
		"    \"expr\": \"role\",\n" +
		"    \"parts\": [\n" +
		"      {\n        \"text\": \"C\",\n        \"type\": \"paragraph\"\n      }\n" +
		"    ],\n" +
		"    \"taken\": \"\\\"admin\\\"\",\n" +
		"    \"type\": \"switch\"\n" +
		"  }\n" +
		"]"
	if got != want {
		t.Fatalf("json nodes:\ngot:\n%s\nwant:\n%s", got, want)
	}
}

func TestNestedListMarkdown(t *testing.T) {
	p := New().Set("env", map[string]any{"shell": "bash"}).Add(
		OrderedList(
			ListItem("**Understand Requirements**: Focus on the requirements provided and apply your assigned perspective throughout the design process."),
			ListItem("**Explore Thoroughly**:",
				List(
					"Read any files provided to you in the initial prompt",
					"Find existing patterns and conventions using `find`, `grep`, and `read`",
					"Understand the current architecture",
					"Identify similar features as reference",
					"Trace through relevant code paths",
				),
				If(`env.shell == "bash"`,
					List(
						"Use {{env.shell}} ONLY for read-only operations (`ls, git status, git log, git diff, find, grep, cat, head, tail`)",
						"NEVER use {{env.shell}} for: mkdir, touch, rm, cp, mv, git add, git commit, npm install, pip install, or any file creation/modification",
					),
				).Else(
					List(
						`Use {{env.shell}} ONLY for read-only operations ("Get-ChildItem, git status, git log, git diff, Get-Content, Select-Object -First/-Last")`,
						"NEVER use {{env.shell}} for: New-Item, Remove-Item, Copy-Item, Move-Item, git add, git commit, npm install, pip install, or any file creation/modification",
					),
				),
			),
			ListItem("**Design Solution**:",
				List(
					"Create implementation approach based on your assigned perspective",
					"Consider trade-offs and architectural decisions",
					"Follow existing patterns where appropriate",
				),
			),
			ListItem("**Detail the Plan**:",
				List(
					"Provide step-by-step implementation strategy",
					"Identify dependencies and sequencing",
					"Anticipate potential challenges",
				),
			),
		),
	)

	want := "1. **Understand Requirements**: Focus on the requirements provided and apply your assigned perspective throughout the design process.\n" +
		"2. **Explore Thoroughly**:\n" +
		"  - Read any files provided to you in the initial prompt\n" +
		"  - Find existing patterns and conventions using `find`, `grep`, and `read`\n" +
		"  - Understand the current architecture\n" +
		"  - Identify similar features as reference\n" +
		"  - Trace through relevant code paths\n" +
		"  - Use bash ONLY for read-only operations (`ls, git status, git log, git diff, find, grep, cat, head, tail`)\n" +
		"  - NEVER use bash for: mkdir, touch, rm, cp, mv, git add, git commit, npm install, pip install, or any file creation/modification\n" +
		"3. **Design Solution**:\n" +
		"  - Create implementation approach based on your assigned perspective\n" +
		"  - Consider trade-offs and architectural decisions\n" +
		"  - Follow existing patterns where appropriate\n" +
		"4. **Detail the Plan**:\n" +
		"  - Provide step-by-step implementation strategy\n" +
		"  - Identify dependencies and sequencing\n" +
		"  - Anticipate potential challenges"

	got, err := p.Build(FormatMarkdown)
	if err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Fatalf("nested list:\ngot:\n%s\nwant:\n%s", got, want)
	}
}

func TestNestedListXMLJSON(t *testing.T) {
	p := New().Add(OrderedList(
		ListItem("A", List("a1", "a2")),
		"B",
	))

	got, err := p.Build(FormatXML)
	if err != nil {
		t.Fatal(err)
	}
	wantXML := "<list ordered=\"true\">\n" +
		"  <item>A\n" +
		"    <list ordered=\"false\">\n" +
		"      <item>a1</item>\n" +
		"      <item>a2</item>\n" +
		"    </list>\n" +
		"  </item>\n" +
		"  <item>B</item>\n" +
		"</list>"
	if got != wantXML {
		t.Fatalf("xml:\ngot:\n%s\nwant:\n%s", got, wantXML)
	}

	got, err = p.Build(FormatJSON)
	if err != nil {
		t.Fatal(err)
	}
	wantJSON := "[\n" +
		"  {\n" +
		"    \"items\": [\n" +
		"      {\n" +
		"        \"children\": [\n" +
		"          {\n            \"items\": [\n              \"a1\",\n              \"a2\"\n            ],\n            \"ordered\": false,\n            \"type\": \"list\"\n          }\n" +
		"        ],\n" +
		"        \"text\": \"A\"\n" +
		"      },\n" +
		"      \"B\"\n" +
		"    ],\n" +
		"    \"ordered\": true,\n" +
		"    \"type\": \"list\"\n" +
		"  }\n" +
		"]"
	if got != wantJSON {
		t.Fatalf("json:\ngot:\n%s\nwant:\n%s", got, wantJSON)
	}
}

func TestListInvalidItemPanics(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Fatal("expected panic for invalid list item")
		}
	}()
	List(42)
}

func TestIfElseArrayNotEmpty(t *testing.T) {
	build := func(tools []string) string {
		items := make([]any, len(tools))
		for i, tool := range tools {
			items[i] = tool
		}
		p := New().
			Set("tools", tools).
			Add(
				If(`len("tools") > 0`,
					Heading(2, "Tools"),
					List(items...),
				).Else(
					Paragraph("(clean)"),
				),
			)
		out, err := p.Build(FormatMarkdown)
		if err != nil {
			t.Fatal(err)
		}
		return out
	}

	if got := build([]string{"grep", "read"}); got != "## Tools\n\n- grep\n- read" {
		t.Fatalf("non-empty: %q", got)
	}
	if got := build([]string{}); got != "(clean)" {
		t.Fatalf("empty: %q", got)
	}
}

func TestDefinedInPrompt(t *testing.T) {
	p := New().
		Set("role", "coder").
		Add(If(`defined("role")`, Paragraph("role set"))).
		Add(If(`!defined("missing")`, Paragraph("missing unset"))).
		Add(If(`defined(missing) == false`, Paragraph("also unset")))
	got, err := p.Build(FormatMarkdown)
	if err != nil {
		t.Fatal(err)
	}
	if got != "role set\n\nmissing unset\n\nalso unset" {
		t.Fatalf("got %q", got)
	}

	v := Vars{"a": 1}
	if !v.Has("a") || v.Has("b") || v.Has("a.b") {
		t.Fatal("Vars.Has misbehaves")
	}
	n := Vars{"n": map[string]any{"m": 2}}
	if !n.Has("n.m") {
		t.Fatal("Vars.Has dotted path failed")
	}
}

func TestRawNoInterpolation(t *testing.T) {
	p := New().Set("name", "Omer").Raw("literal {{name}}")
	got, err := p.Build(FormatMarkdown)
	if err != nil {
		t.Fatal(err)
	}
	if got != "literal {{name}}" {
		t.Fatalf("got %q", got)
	}
}

func TestHeadingLevelClamp(t *testing.T) {
	for _, tc := range []struct{ level, want int }{
		{1, 1}, {3, 3}, {9, 6}, {0, 1}, {-2, 1},
	} {
		p := New().Add(Heading(tc.level, "h"))
		if tc.want != 1 {
			continue
		}
		got, err := p.Build(FormatXML)
		if err != nil {
			t.Fatal(err)
		}
		want := `<heading level="1">h</heading>`
		if got != want {
			t.Fatalf("level=%d: got %q want %q", tc.level, got, want)
		}
	}
	p := New().Add(Heading(7, "h"))
	got, err := p.Build(FormatXML)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(got, `level="7"`) {
		t.Fatalf("level 7 was not clamped: %q", got)
	}
}

func TestAgentsMd(t *testing.T) {
	t.Run("with content", func(t *testing.T) {
		dir := t.TempDir()
		if err := os.WriteFile(dir+"/AGENTS.md", []byte("# Rules\nBe concise."), 0o644); err != nil {
			t.Fatal(err)
		}
		orig, _ := os.Getwd()
		os.Chdir(dir)
		defer os.Chdir(orig)

		p := New().
			Paragraph("before").
			AgentsMd().
			Paragraph("after")

		got, err := p.Build(FormatMarkdown)
		if err != nil {
			t.Fatal(err)
		}
		want := "before\n\n# Project Instructions\n\n# Rules\nBe concise.\n\nafter"
		if got != want {
			t.Fatalf("markdown:\ngot:\n%s\nwant:\n%s", got, want)
		}
	})

	t.Run("file not found", func(t *testing.T) {
		dir := t.TempDir()
		orig, _ := os.Getwd()
		os.Chdir(dir)
		defer os.Chdir(orig)

		p := New().
			Paragraph("before").
			AgentsMd().
			Paragraph("after")

		got, err := p.Build(FormatMarkdown)
		if err != nil {
			t.Fatal(err)
		}
		want := "before\n\nafter"
		if got != want {
			t.Fatalf("markdown:\ngot:\n%s\nwant:\n%s", got, want)
		}
	})

	t.Run("empty file", func(t *testing.T) {
		dir := t.TempDir()
		if err := os.WriteFile(dir+"/AGENTS.md", []byte(""), 0o644); err != nil {
			t.Fatal(err)
		}
		orig, _ := os.Getwd()
		os.Chdir(dir)
		defer os.Chdir(orig)

		p := New().
			Paragraph("before").
			AgentsMd().
			Paragraph("after")

		got, err := p.Build(FormatMarkdown)
		if err != nil {
			t.Fatal(err)
		}
		want := "before\n\nafter"
		if got != want {
			t.Fatalf("markdown:\ngot:\n%s\nwant:\n%s", got, want)
		}
	})
}

func TestRules(t *testing.T) {
	t.Run("global and project rules", func(t *testing.T) {
		homeDir := t.TempDir()
		rulesDir := filepath.Join(homeDir, ".pegg", "rules")
		if err := os.MkdirAll(rulesDir, 0o755); err != nil {
			t.Fatal(err)
		}

		if err := os.WriteFile(filepath.Join(rulesDir, "01-global.md"), []byte("# Global Rule\nAlways be concise."), 0o644); err != nil {
			t.Fatal(err)
		}

		projectDir := t.TempDir()
		projectRulesDir := filepath.Join(projectDir, ".pegg", "rules")
		if err := os.MkdirAll(projectRulesDir, 0o755); err != nil {
			t.Fatal(err)
		}

		if err := os.WriteFile(filepath.Join(projectRulesDir, "02-project.md"), []byte("# Project Rule\nFollow conventions."), 0o644); err != nil {
			t.Fatal(err)
		}

		orig, _ := os.Getwd()
		os.Chdir(projectDir)
		defer os.Chdir(orig)

		t.Setenv("HOME", homeDir)

		p := New().
			Paragraph("before").
			Rules().
			Paragraph("after")

		got, err := p.Build(FormatMarkdown)
		if err != nil {
			t.Fatal(err)
		}

		if !strings.Contains(got, "# Rules") {
			t.Fatal("missing Rules heading")
		}
		if !strings.Contains(got, "WARNING") {
			t.Fatal("missing stern warning")
		}
		if !strings.Contains(got, "# Global Rule") {
			t.Fatal("missing global rule content")
		}
		if !strings.Contains(got, "# Project Rule") {
			t.Fatal("missing project rule content")
		}
	})

	t.Run("no rules directories", func(t *testing.T) {
		dir := t.TempDir()
		orig, _ := os.Getwd()
		os.Chdir(dir)
		defer os.Chdir(orig)

		p := New().
			Paragraph("before").
			Rules().
			Paragraph("after")

		got, err := p.Build(FormatMarkdown)
		if err != nil {
			t.Fatal(err)
		}
		want := "before\n\nafter"
		if got != want {
			t.Fatalf("markdown:\ngot:\n%s\nwant:\n%s", got, want)
		}
	})

	t.Run("empty rules directory", func(t *testing.T) {
		dir := t.TempDir()
		rulesDir := filepath.Join(dir, ".pegg", "rules")
		if err := os.MkdirAll(rulesDir, 0o755); err != nil {
			t.Fatal(err)
		}

		orig, _ := os.Getwd()
		os.Chdir(dir)
		defer os.Chdir(orig)

		p := New().
			Paragraph("before").
			Rules().
			Paragraph("after")

		got, err := p.Build(FormatMarkdown)
		if err != nil {
			t.Fatal(err)
		}
		want := "before\n\nafter"
		if got != want {
			t.Fatalf("markdown:\ngot:\n%s\nwant:\n%s", got, want)
		}
	})
}
