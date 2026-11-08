package main

import (
	"fmt"
	"os"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/charmbracelet/bubbles/textarea"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
)

// Item-screen fields, in tab order (Pick is skipped when the item lists no
// options).
const (
	dfPick = iota
	dfAnswer
	dfOutcome
	dfSave
)

type decideMode int

const (
	modeList decideMode = iota
	modeItem
)

type commentsMsg struct {
	id       string
	comments []comment
	err      error
}

type savedMsg struct {
	idx   int
	steps []step
	done  int
	err   error
}

type decideModel struct {
	items   []decision
	handled map[int]bool // answered in this session
	dryRun  bool
	mode    decideMode
	cursor  int // list cursor
	width   int
	height  int

	// Item screen.
	cur       int
	opts      []decisionOption
	pickIdx   int
	picked    int // option whose text filled the answer, -1 none
	answer    textarea.Model
	outcome   decideOutcome
	focus     int
	vp        viewport.Model
	comments  []comment
	commentsS string // "", "loading", or an error message
	saving    bool
	errMsg    string
	status    string

	// Session results, reported after the TUI exits.
	saved   int
	planned []string
	notes   []string
}

func newDecideModel(items []decision, dryRun bool) decideModel {
	ta := textarea.New()
	ta.Placeholder = "Your answer — it is saved as a comment on the item…"
	ta.ShowLineNumbers = false
	ta.SetWidth(72)
	ta.SetHeight(3)
	return decideModel{
		items: items, handled: map[int]bool{}, dryRun: dryRun,
		answer: ta, picked: -1, outcome: outcomeAnswered,
		vp: viewport.New(72, 6), width: 80, height: 24,
	}
}

func (m decideModel) Init() tea.Cmd { return nil }

// --- layout ------------------------------------------------------------

func (m *decideModel) fields() []int {
	if len(m.opts) > 0 {
		return []int{dfPick, dfAnswer, dfOutcome, dfSave}
	}
	return []int{dfAnswer, dfOutcome, dfSave}
}

func (m *decideModel) setFocus(f int) {
	m.focus = f
	if f == dfAnswer {
		m.answer.Focus()
	} else {
		m.answer.Blur()
	}
}

func (m *decideModel) moveFocus(dir int) {
	fs := m.fields()
	for i, f := range fs {
		if f == m.focus {
			m.setFocus(fs[(i+dir+len(fs))%len(fs)])
			return
		}
	}
	m.setFocus(fs[0])
}

// relayout sizes the answer box and gives the reading pane whatever height
// the header and the answer area leave.
func (m *decideModel) relayout() {
	m.answer.SetWidth(max(24, min(76, m.width-8)))
	bodyW := max(20, m.width-6)
	if m.vp.Width != bodyW {
		m.vp.Width = bodyW
		m.refreshBody()
	}
	used := strings.Count(m.itemHeader(), "\n") + 1 + strings.Count(m.itemBottom(), "\n") + 2
	m.vp.Height = max(3, m.height-used)
}

func (m *decideModel) refreshBody() {
	if m.cur < 0 || m.cur >= len(m.items) {
		return
	}
	m.vp.SetContent(renderDecisionBody(m.items[m.cur], m.comments, m.commentsS, m.vp.Width, activeTheme()))
}

// --- item screen -------------------------------------------------------

func (m *decideModel) openItem(i int) tea.Cmd {
	m.mode = modeItem
	m.cur = i
	d := m.items[i]
	m.opts = parseOptions(d.Description)
	m.pickIdx, m.picked = 0, -1
	for j, o := range m.opts {
		if o.Recommended {
			m.pickIdx = j
			break
		}
	}
	m.answer.Reset()
	m.outcome = outcomeAnswered
	m.errMsg = ""
	m.comments, m.commentsS = nil, ""
	m.vp.GotoTop()
	var cmd tea.Cmd
	if d.CommentCount > 0 || m.handled[i] {
		m.commentsS = "loading"
		cmd = fetchCommentsCmd(d)
	}
	if len(m.opts) > 0 {
		m.setFocus(dfPick)
	} else {
		m.setFocus(dfAnswer)
	}
	m.vp.Width = 0 // force a body refresh in relayout
	m.relayout()
	return cmd
}

func fetchCommentsCmd(d decision) tea.Cmd {
	return func() tea.Msg {
		out, err := runStoreJSON(d.Store, commentsArgv(d.ID))
		if err != nil {
			return commentsMsg{id: d.ID, err: err}
		}
		cs, err := parseComments(out)
		return commentsMsg{id: d.ID, comments: cs, err: err}
	}
}

func (m *decideModel) choose(i int) {
	m.picked = i
	m.pickIdx = i
	m.answer.SetValue(optionAnswer(m.opts[i]))
	m.answer.CursorEnd()
	m.errMsg = ""
	m.setFocus(dfAnswer)
}

// save validates, then runs the comment (and label/close) steps. An answer
// is always required, whatever the outcome.
func (m decideModel) save() (tea.Model, tea.Cmd) {
	text := strings.TrimSpace(m.answer.Value())
	if text == "" {
		m.errMsg = "an answer is required — type one or pick an option"
		m.setFocus(dfAnswer)
		return m, nil
	}
	d := m.items[m.cur]
	steps := saveSteps(d, m.outcome, text, time.Now())
	m.errMsg = ""
	idx := m.cur
	if m.dryRun {
		return m, func() tea.Msg { return savedMsg{idx: idx, steps: steps, done: len(steps)} }
	}
	m.saving = true
	return m, func() tea.Msg {
		done, err := runSteps(d.Store, steps)
		return savedMsg{idx: idx, steps: steps, done: done, err: err}
	}
}

// nextOpen is the next item after from still waiting on an answer, wrapping
// around; -1 when none is left.
func (m decideModel) nextOpen(from int) int {
	for k := 1; k <= len(m.items); k++ {
		i := (from + k) % len(m.items)
		if !m.handled[i] && !m.items[i].answered() {
			return i
		}
	}
	return -1
}

func (m decideModel) onSaved(msg savedMsg) (tea.Model, tea.Cmd) {
	m.saving = false
	d := m.items[msg.idx]
	if msg.done == 0 && msg.err != nil {
		m.errMsg = msg.err.Error()
		return m, nil
	}
	m.handled[msg.idx] = true
	m.saved++
	m.status = fmt.Sprintf("✓ saved answer on %s", d.ID)
	if m.dryRun {
		m.status = fmt.Sprintf("✓ dry run: would answer %s", d.ID)
		for _, s := range msg.steps {
			m.planned = append(m.planned, formatStep(d.Store, s))
		}
	}
	if msg.err != nil {
		// The answer is recorded; only the follow-up step failed. Retrying
		// would duplicate the comment, so move on and say so.
		note := fmt.Sprintf("%s: answer saved, but %v", d.ID, msg.err)
		m.notes = append(m.notes, note)
		m.status = "⚠ " + note
	}
	next := m.nextOpen(msg.idx)
	if next < 0 {
		return m, tea.Quit
	}
	m.cursor = next
	return m, m.openItem(next)
}

func (m decideModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		m.vp.Width = 0
		m.relayout()
		return m, nil
	case commentsMsg:
		if m.mode == modeItem && m.items[m.cur].ID == msg.id {
			m.comments, m.commentsS = msg.comments, ""
			if msg.err != nil {
				m.commentsS = "couldn't load comments: " + msg.err.Error()
			}
			m.refreshBody()
		}
		return m, nil
	case savedMsg:
		return m.onSaved(msg)
	case tea.KeyMsg:
		if m.saving {
			if msg.String() == "ctrl+c" {
				return m, tea.Quit
			}
			return m, nil
		}
		if m.mode == modeList {
			return m.updateList(msg)
		}
		next, cmd := m.updateItem(msg)
		if dm, ok := next.(decideModel); ok {
			dm.relayout()
			return dm, cmd
		}
		return next, cmd
	}
	if m.mode == modeItem && m.focus == dfAnswer {
		var cmd tea.Cmd
		m.answer, cmd = m.answer.Update(msg)
		return m, cmd
	}
	return m, nil
}

func (m decideModel) updateList(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc", "ctrl+c":
		return m, tea.Quit
	case "enter":
		m.status = ""
		return m, m.openItem(m.cursor)
	case "up", "ctrl+p":
		if m.cursor > 0 {
			m.cursor--
		}
	case "down", "ctrl+n":
		if m.cursor < len(m.items)-1 {
			m.cursor++
		}
	case "pgup":
		m.cursor = max(0, m.cursor-visibleRows(m.height, len(m.items)))
	case "pgdown":
		m.cursor = min(len(m.items)-1, m.cursor+visibleRows(m.height, len(m.items)))
	case "home":
		m.cursor = 0
	case "end":
		m.cursor = len(m.items) - 1
	}
	return m, nil
}

func (m decideModel) updateItem(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "ctrl+c":
		return m, tea.Quit
	case "esc":
		m.mode = modeList
		m.errMsg = ""
		return m, nil
	case "ctrl+s":
		return m.save()
	case "ctrl+n", "ctrl+p":
		step := 1
		if msg.String() == "ctrl+p" {
			step = -1
		}
		m.cursor = (m.cur + step + len(m.items)) % len(m.items)
		m.status = ""
		return m, m.openItem(m.cursor)
	case "pgup", "alt+up":
		m.vp.PageUp()
		return m, nil
	case "pgdown", "alt+down":
		m.vp.PageDown()
		return m, nil
	case "tab", "down":
		m.errMsg = ""
		m.moveFocus(1)
		return m, nil
	case "shift+tab", "up":
		m.errMsg = ""
		m.moveFocus(-1)
		return m, nil
	}

	switch m.focus {
	case dfPick:
		switch msg.String() {
		case "left":
			if m.pickIdx > 0 {
				m.pickIdx--
			}
		case "right":
			if m.pickIdx < len(m.opts)-1 {
				m.pickIdx++
			}
		case "enter":
			m.choose(m.pickIdx)
		default:
			if msg.Type == tea.KeyRunes && len(msg.Runes) == 1 {
				key := strings.ToLower(string(msg.Runes))
				for i, o := range m.opts {
					if o.Key == key {
						m.choose(i)
						break
					}
				}
			}
		}
		return m, nil
	case dfOutcome:
		switch msg.String() {
		case "left":
			if m.outcome > 0 {
				m.outcome--
			}
		case "right":
			if m.outcome < outcomeCount-1 {
				m.outcome++
			}
		}
		return m, nil
	case dfSave:
		if msg.String() == "enter" {
			return m.save()
		}
		return m, nil
	}
	var cmd tea.Cmd
	m.answer, cmd = m.answer.Update(msg)
	return m, cmd
}

// --- views -------------------------------------------------------------

func (m decideModel) View() string {
	if m.mode == modeList {
		return m.listView()
	}
	return m.itemHeader() + "\n" + m.vp.View() + "\n" + m.scrollHint() + "\n" + m.itemBottom()
}

func (m decideModel) heading(text string) string {
	th := activeTheme()
	if m.dryRun {
		text += "  " + th.Dim.Render("[dry run]")
	}
	return th.Title.Render(text)
}

func (m decideModel) listView() string {
	th := activeTheme()
	var b strings.Builder
	n := len(m.items)
	left := 0
	for i, d := range m.items {
		if !m.handled[i] && !d.answered() {
			left++
		}
	}
	fmt.Fprintf(&b, "%s\n", m.heading(fmt.Sprintf("Decisions waiting on you  %d", left)))
	if m.status != "" {
		fmt.Fprintf(&b, "  %s\n", th.Dim.Render(m.status))
	} else {
		fmt.Fprintf(&b, "\n")
	}

	maxStore, maxID := 0, 0
	for _, d := range m.items {
		maxStore = max(maxStore, len(d.Store))
		maxID = max(maxID, len(d.ID))
	}
	rows := visibleRows(m.height-1, n)
	start := 0
	if n > rows {
		start = min(max(0, m.cursor-rows/2), n-rows)
	}
	end := min(n, start+rows)
	if start > 0 {
		fmt.Fprintf(&b, "  %s\n", th.Dim.Render(fmt.Sprintf("▲ %d more", start)))
	}
	for i := start; i < end; i++ {
		d := m.items[i]
		mark := " "
		if m.handled[i] || d.answered() {
			mark = "✓"
		}
		prefix := fmt.Sprintf("%s P%d  %s  %s  ", mark, d.Priority, padRight(d.Store, maxStore), padRight(d.ID, maxID))
		title := shorten(d.Title, max(20, m.width-utf8.RuneCountInString(prefix)-4))
		if i == m.cursor {
			fmt.Fprintf(&b, "%s\n", th.Reverse.Render("  "+prefix+title))
		} else {
			fmt.Fprintf(&b, "  %s%s\n", th.Dim.Render(prefix), th.Title.Render(title))
		}
	}
	if end < n {
		fmt.Fprintf(&b, "  %s\n", th.Dim.Render(fmt.Sprintf("▼ %d more", n-end)))
	}
	fmt.Fprintf(&b, "\n  %s\n", th.Dim.Render("↑↓ move · ⏎ open · esc quit"))
	return b.String()
}

func (m decideModel) itemHeader() string {
	th := activeTheme()
	d := m.items[m.cur]
	h := m.heading(fmt.Sprintf("Decision %d/%d  %s", m.cur+1, len(m.items), th.Name.Render(d.ID)))
	line := "  " + th.Dim.Render(" ")
	switch {
	case m.errMsg != "":
		line = "  " + th.Danger.Render("⚠ "+m.errMsg)
	case m.saving:
		line = "  " + th.Dim.Render("saving…")
	case m.status != "":
		line = "  " + th.Dim.Render(m.status)
	}
	return h + "\n" + line
}

func (m decideModel) scrollHint() string {
	th := activeTheme()
	if m.vp.AtTop() && m.vp.AtBottom() {
		return " "
	}
	return "  " + th.Dim.Render(fmt.Sprintf("pgup/pgdn scroll · %d%%", int(m.vp.ScrollPercent()*100)))
}

func (m decideModel) itemBottom() string {
	th := activeTheme()
	var b strings.Builder
	marker := func(f int) string {
		if m.focus == f {
			return th.Accent.Render("❯")
		}
		return " "
	}
	fs := m.fields()
	at := indexOf(fs, m.focus)
	navHint := func(f int) string {
		switch f {
		case fs[(at+1)%len(fs)]:
			return "  " + th.Hint.Render("-- TAB --")
		case fs[(at+len(fs)-1)%len(fs)]:
			return "  " + th.Hint.Render("-- SHIFT+TAB --")
		}
		return ""
	}

	if len(m.opts) > 0 {
		fmt.Fprintf(&b, "  %s %s  %s%s\n", marker(dfPick), th.Title.Render("Pick"),
			th.Dim.Render("press a key, ←/→ + ⏎"), navHint(dfPick))
		for i, o := range m.opts {
			key := o.Key
			if key == "" {
				key = " "
			}
			line := fmt.Sprintf("[%s] %s", key, shorten(o.Name, max(20, m.width-16)))
			if o.Recommended {
				line += "  ★"
			}
			switch {
			case m.focus == dfPick && i == m.pickIdx:
				fmt.Fprintf(&b, "      %s\n", th.Reverse.Render(" "+line+" "))
			case i == m.picked:
				fmt.Fprintf(&b, "      %s\n", th.Name.Render(" "+line))
			default:
				fmt.Fprintf(&b, "      %s\n", th.Dim.Render(" "+line))
			}
		}
	}

	fmt.Fprintf(&b, "  %s %s%s\n", marker(dfAnswer), th.Title.Render("Answer"), navHint(dfAnswer))
	fmt.Fprintf(&b, "    %s\n", strings.ReplaceAll(m.answer.View(), "\n", "\n    "))

	outcome := th.Name.Render(m.outcome.String())
	hint := m.outcomeHint()
	if m.outcome == outcomeClose {
		outcome = th.Danger.Render(m.outcome.String())
		hint = th.Danger.Render(hint)
	} else {
		hint = th.Dim.Render(hint)
	}
	fmt.Fprintf(&b, "  %s %s  %s %s %s  %s%s\n", marker(dfOutcome), th.Title.Render("After saving"),
		th.Dim.Render("⟨"), outcome, th.Dim.Render("⟩"), hint, navHint(dfOutcome))

	btn := th.Dim.Render("[ Save answer ]")
	if m.focus == dfSave {
		btn = th.Reverse.Render(" Save answer ")
	}
	fmt.Fprintf(&b, "  %s %s%s\n", marker(dfSave), btn, navHint(dfSave))

	fmt.Fprintf(&b, "  %s", th.Dim.Render(
		"tab/↑↓ fields · ←/→ pick/after saving · ctrl+s save · ctrl+n/p skip · esc list"))
	return b.String()
}

func (m decideModel) outcomeHint() string {
	switch m.outcome {
	case outcomeComment:
		return "leave the item untouched"
	case outcomeClose:
		return "closes it for everyone — only if you mean it"
	default:
		return "adds the " + answeredLabel + " label; stays open"
	}
}

func indexOf(xs []int, v int) int {
	for i, x := range xs {
		if x == v {
			return i
		}
	}
	return 0
}

// --- reading pane ------------------------------------------------------

// wrapText word-wraps s to width, keeping its line breaks and giving
// bullet/numbered lines a hanging indent.
func wrapText(s string, width int) string {
	var out []string
	for _, line := range strings.Split(strings.TrimRight(s, "\n"), "\n") {
		line = strings.TrimRight(line, " \t\r")
		trimmed := strings.TrimLeft(line, " ")
		indent := len(line) - len(trimmed)
		hang := indent
		if m := bulletRe.FindStringSubmatch(trimmed); m != nil {
			hang += len(trimmed) - len(m[1])
		} else if m := labelRe.FindStringSubmatch(trimmed); m != nil {
			hang += len(trimmed) - len(m[2])
		}
		if utf8.RuneCountInString(line) <= width || width < hang+10 {
			out = append(out, line)
			continue
		}
		cur := strings.Repeat(" ", indent)
		curLen := indent
		first := true
		for _, w := range strings.Fields(trimmed) {
			wl := utf8.RuneCountInString(w)
			if !first && curLen+1+wl > width {
				out = append(out, cur)
				cur = strings.Repeat(" ", hang) + w
				curLen = hang + wl
				continue
			}
			if !first {
				cur += " "
				curLen++
			}
			cur += w
			curLen += wl
			first = false
		}
		out = append(out, cur)
	}
	return strings.Join(out, "\n")
}

func shortDate(ts string) string {
	if t, err := time.Parse(time.RFC3339, ts); err == nil {
		return t.Local().Format("2006-01-02 15:04")
	}
	return ts
}

// renderDecisionBody lays out the whole item for reading: title, meta, the
// description (its headings picked out), links, and the comment thread.
func renderDecisionBody(d decision, comments []comment, commentsState string, width int, th *Theme) string {
	var b strings.Builder
	fmt.Fprintf(&b, "%s\n", th.Title.Render(wrapText(d.Title, width)))
	meta := fmt.Sprintf("P%d · %s · opened %s", d.Priority, d.Store, shortDate(d.CreatedAt))
	if len(d.Labels) > 0 {
		meta += " · " + strings.Join(d.Labels, ", ")
	}
	fmt.Fprintf(&b, "%s\n\n", th.Dim.Render(wrapText(meta, width)))

	desc := strings.TrimSpace(strings.ReplaceAll(d.Description, "\r\n", "\n"))
	if desc == "" {
		fmt.Fprintf(&b, "%s\n", th.Dim.Render("(no description)"))
	}
	for _, line := range strings.Split(wrapText(desc, width), "\n") {
		if desc != "" && isHeading(line) {
			line = th.Accent.Render(line)
		}
		fmt.Fprintf(&b, "%s\n", line)
	}

	if links := itemLinks(d.Description); len(links) > 0 {
		fmt.Fprintf(&b, "\n%s\n", th.Accent.Render("LINKS"))
		for _, l := range links {
			fmt.Fprintf(&b, "  %s\n", l)
		}
	}

	switch {
	case commentsState == "loading":
		fmt.Fprintf(&b, "\n%s\n  %s\n", th.Accent.Render("COMMENTS"), th.Dim.Render("loading…"))
	case commentsState != "":
		fmt.Fprintf(&b, "\n%s\n  %s\n", th.Accent.Render("COMMENTS"), th.Danger.Render(commentsState))
	case len(comments) > 0:
		fmt.Fprintf(&b, "\n%s\n", th.Accent.Render(fmt.Sprintf("COMMENTS (%d)", len(comments))))
		for _, c := range comments {
			fmt.Fprintf(&b, "  %s\n", th.Dim.Render(strings.TrimSpace(c.Author+" · "+shortDate(c.CreatedAt))))
			for _, l := range strings.Split(wrapText(c.Text, width-4), "\n") {
				fmt.Fprintf(&b, "    %s\n", l)
			}
		}
	default:
		fmt.Fprintf(&b, "\n%s\n  %s\n", th.Accent.Render("COMMENTS"), th.Dim.Render("none yet"))
	}
	return b.String()
}

// runDecideTUI runs the decision queue and reports the session afterwards.
func runDecideTUI(items []decision, dryRun bool) int {
	p := tea.NewProgram(newDecideModel(items, dryRun), tea.WithAltScreen())
	final, err := p.Run()
	if err != nil {
		fmt.Fprintf(os.Stderr, "bead: decide unavailable (%v)\n", err)
		return 1
	}
	m, ok := final.(decideModel)
	if !ok {
		return 1
	}
	if m.dryRun {
		for _, l := range m.planned {
			fmt.Fprintf(os.Stderr, "bead: dry run would run: %s\n", l)
		}
		fmt.Fprintf(os.Stderr, "bead: dry run — nothing written.\n")
	} else if m.saved > 0 {
		fmt.Fprintf(os.Stderr, "bead: saved %d answer(s).\n", m.saved)
	}
	for _, n := range m.notes {
		fmt.Fprintf(os.Stderr, "bead: warning: %s\n", n)
	}
	if m.nextOpen(-1) < 0 && len(items) > 0 {
		fmt.Fprintf(os.Stderr, "bead: all caught up — no decisions waiting on you.\n")
	}
	if len(m.notes) > 0 {
		return 1
	}
	return 0
}
