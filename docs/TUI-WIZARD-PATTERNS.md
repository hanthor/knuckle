# TUI Wizard Patterns for Knuckle

> Research into charm.sh/bubbletea multi-step wizard patterns, with concrete
> recommendations for making knuckle feel like a **professional installer**, not
> a series of forms.

---

## 1. What We Learned From the Charm Ecosystem

### 1.1  huh.Form — Group Transition Mechanics

**Source:** `charmbracelet/huh@v1.0.0` — `group.go`, `form.go`

huh groups transition via two internal message types:

```go
type nextGroupMsg struct{}
type prevGroupMsg struct{}
```

When the *last* field in a group sends `NextField`, the group sends `nextGroup`.
The form's `Update` handles `nextGroupMsg` by:

1. **Validating the current group first** — if `len(group.Errors()) > 0`, the
   transition is blocked and errors are shown in the group footer.
2. **Hard-switching** to the next non-hidden group — `f.selector.SetIndex(i)`.
3. Calling `f.selector.Selected().Init()` which fires `Focus()` on the first
   field of the new group.

**Key insight:** There is **no built-in animation** between groups. The
transition is instantaneous. The only visual feedback is:
- The focused field gets a thick left border
  (`lipgloss.ThickBorder().BorderLeft(true)`)
- The blurred fields get a hidden border (`lipgloss.HiddenBorder()`)
- Validation errors appear in the group footer *before* the tab advance is
  accepted

**For knuckle:** The between-step progress indicator *is* the animation. huh
gives you group-level transitions for free; the outer wizard chrome signals
"you moved forward".

---

### 1.2  huh Focused/Blurred Field Visual Language

Every theme uses this two-state contract for fields:

| State | Left border | Description style | Cursor colour |
|---|---|---|---|
| **Focused** | Thick coloured bar | dim/subtext | accent (fuchsia/green/yellow) |
| **Blurred** | Hidden (invisible) | subdued | plain |

In ThemeDracula (what knuckle uses):
```
Focused border foreground: "#44475a" (selection)
Focused title:             "#bd93f9" (purple)
Error indicator:           "#ff5555" (red)
Select selector:           "#f1fa8c" (yellow)
Text input cursor:         "#f1fa8c" (yellow)
```

The thick left border on the active field is the *primary affordance* that
tells the user which field is selected. This is more legible than cursor-only
approaches.

---

### 1.3  Burger Example — Multi-Group Pattern

The canonical huh multi-group pattern (burger example):

```go
huh.NewForm(
    huh.NewGroup(         // Page 1: what to order
        huh.NewSelect[string]()...,
        huh.NewMultiSelect[string]()...,
        huh.NewSelect[int]()...,
    ),
    huh.NewGroup(         // Page 2: who's ordering
        huh.NewInput()...Validate(...),
        huh.NewText()...,
        huh.NewConfirm()...,
    ),
)
```

Groups within one `huh.Form` share the same theme and key map. The form
advances automatically when the last field in a group sends `NextField`. The
caller doesn't manage transitions — `huh.StateCompleted` fires only after the
*last* group.

**knuckle already does this correctly** for the User step (two groups:
"System Identity" + "Authentication"). Extend this pattern.

---

### 1.4  Validation in huh

Each field accepts `.Validate(func(T) error)`. When the user tries to advance:
- `Blur()` is called on the focused field
- `Blur()` sets `field.err = field.validate(field.accessor.Get())`
- The group's `Footer()` renders all non-nil errors in red
- `nextGroupMsg` is blocked if `len(group.Errors()) > 0`

The error message appears *in the footer of the current group*, not inline with
the field. This is different from what knuckle currently does (top-level `m.err`).

**Recommendation:** Move validation into `huh.Validate()` callbacks on each
field. The current pattern of checking errors in `onFormComplete()` runs
*after* form completion — too late for incremental feedback.

---

### 1.5  bubbles/progress — Spring-Animated Bar

**Source:** `charmbracelet/bubbles@v0.21.1` — `progress/progress.go`

The progress bar uses **harmonica spring physics** for smooth animation:

```go
m.percentShown, m.velocity = m.spring.Update(m.percentShown, m.velocity, m.targetPercent)
```

Default: frequency=18, damping=1 (critically damped, no bounce). It fires
`FrameMsg` at 60fps until `percentShown ≈ targetPercent`.

```go
bar := progress.New(
    progress.WithGradient("#5A56E0", "#EE6FF8"),  // indigo → pink
    // OR for a Flatcar feel:
    progress.WithGradient("#50fa7b", "#ff79c6"),  // Dracula green → pink
    progress.WithWidth(40),
)

// Advance the bar:
cmds = append(cmds, bar.SetPercent(0.67))  // returns a Cmd that drives animation
```

The `FrameMsg` must be forwarded to the bar in your `Update()`:
```go
case progress.FrameMsg:
    progressModel, cmd := m.bar.Update(msg)
    m.bar = progressModel.(progress.Model)
    cmds = append(cmds, cmd)
```

**Knuckle's current install progress** uses a manually constructed
`strings.Repeat("█", filled)` bar with no animation. This is the biggest
visual regression vs professional tools.

---

### 1.6  bubbles/spinner — Current-Phase Indicator

Seven useful spinners:

| Name | Frames | FPS | Best for |
|---|---|---|---|
| `spinner.Dot` | `⣾ ⣽ ⣻ ⢿ ⡿ ⣟ ⣯ ⣷` | 10 | general "working" |
| `spinner.MiniDot` | `⠋⠙⠹⠸⼼⠴⠦⠧⠇⠏` | 12 | compact inline |
| `spinner.Jump` | `⢄⢂⢁⡁⡈⡐⡠` | 10 | lively |
| `spinner.Meter` | `▱▱▱ → ▰▰▰` | 7 | activity meter feel |
| `spinner.Pulse` | `█▓▒░` | 8 | pulsing block (dramatic) |
| `spinner.Ellipsis` | `… .  ..  ...` | 3 | subtle waiting |

Knuckle currently uses raw Unicode spinner chars (`⣷`) without the
`bubbles/spinner` model — it never updates. You need a proper spinner model.

---

### 1.7  bubbles/table — Professional Disk Picker

**Source:** `charmbracelet/bubbles` — `table/` package

The `bubbles/table` model provides:
- Column headers with configurable widths
- Row selection with highlight style
- Keyboard navigation (↑/↓/j/k, home/end, pgup/pgdn)
- Full styling via `table.Styles`

```go
cols := []table.Column{
    {Title: "Device", Width: 20},
    {Title: "Model", Width: 28},
    {Title: "Size", Width: 8},
    {Title: "Type", Width: 8},
    {Title: "⚠", Width: 4},  // removable flag
}

rows := []table.Row{
    {"/dev/disk/by-id/ata-...", "SAMSUNG SSD 860", "500G", "SATA", ""},
    {"/dev/disk/by-id/usb-...", "SanDisk Cruzer",  " 32G", "USB",  "⚠"},
}

t := table.New(
    table.WithColumns(cols),
    table.WithRows(rows),
    table.WithFocused(true),
    table.WithHeight(6),
)
```

**Knuckle's current Storage step** uses a manual cursor loop rendering
`▸` next to disk entries. Migrating to `bubbles/table` gives scroll, header
alignment, and selection highlight for free.

---

## 2. Professional Installer UX Patterns

### 2.1  Three-Zone Layout (Ubuntu Server / Subiquity Model)

Every step in a professional installer has the same chrome:

```
╭──────────────────────────────────────────────────────╮
│  🔧  Knuckle — Flatcar Container Linux Installer     │  ← Title bar (1 line)
╰──────────────────────────────────────────────────────╯
  ① Welcome  ② Network  ③ Disk  ④ User  ⑤ Sysext  ⑥ Review   ← Step rail

  [STEP CONTENT]                                              ← Body (flex)

  tab next field • shift+tab previous • enter confirm • esc back  ← Help bar
```

The **title bar** and **step rail** are fixed. The body changes. The help bar
updates contextually (huh does this automatically via `KeyBinds()`).

**Current knuckle:** Title + progress bar are re-rendered inline on every
`viewWithForm()`, which works but doesn't feel persistent. Consider wrapping in
`lipgloss.NewStyle().Border(lipgloss.RoundedBorder())` for the title card.

---

### 2.2  Step Rail Design

The current breadcrumb (`● Channel  ○ Network  ○ Disk...`) is functional but
compressed. Recommendations:

**Option A — Numbered badges (current + done highlighted):**
```
  ✓ 1  ✓ 2  ● 3  ○ 4  ○ 5  ○ 6  ○ 7
   Ch   Net  Disk  User Ext  Upd  Rev
```

**Option B — Named steps with separator arrows:**
```
  ✓ Channel › ✓ Network › ● Disk › ○ User › ○ Review › ○ Install
```

**Option C — Top progress bar + step name (most compact):**
```
  ━━━━━━━━━━━━━━━━━━━━━━━━████░░░░░  Step 3 of 8 — Storage
```

**Recommendation for knuckle:** Option A. The icons (✓/●/○) are already there.
Add a subtle dim separator (`  ·  `) between steps. Make the current step bold
+ accent colour. Keep it to one line.

```go
func (m *Model) renderStepRail() string {
    steps := []struct{ short string; step model.WizardStep }{
        {"Channel", model.StepWelcome},
        {"Network", model.StepNetwork},
        {"Disk",    model.StepStorage},
        {"User",    model.StepUser},
        {"Sysext",  model.StepSysext},
        {"Update",  model.StepUpdate},
        {"Review",  model.StepReview},
        {"Install", model.StepInstall},
    }
    current := m.Wizard.State.CurrentStep

    doneStyle    := lipgloss.NewStyle().Foreground(lipgloss.Color("#50fa7b"))  // Dracula green
    activeStyle  := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#ff79c6"))  // pink
    futureStyle  := lipgloss.NewStyle().Foreground(lipgloss.Color("#6272a4"))  // Dracula comment
    sepStyle     := lipgloss.NewStyle().Foreground(lipgloss.Color("#44475a"))

    var parts []string
    for _, s := range steps {
        var rendered string
        if s.step < current {
            rendered = doneStyle.Render("✓ " + s.short)
        } else if s.step == current {
            rendered = activeStyle.Render("● " + s.short)
        } else {
            rendered = futureStyle.Render("○ " + s.short)
        }
        parts = append(parts, rendered)
    }
    return strings.Join(parts, sepStyle.Render("  ·  "))
}
```

---

### 2.3  The Confirmation / Destructive Action Screen

This is the **most important screen** in an installer. It must communicate:
1. What will be destroyed (disk path, model, size)
2. That this is irreversible
3. Require deliberate user action (not just Enter)

**What huh.Confirm already gives you:**
```
Title:       "⚠ ALL DATA ON /dev/sda (Samsung 860 EVO, 500 GB) WILL BE DESTROYED"
Description: [summary block]
Affirmative: "Yes, install Flatcar"
Negative:    "No, go back"
```

Button rendering:
```
  [ Yes, install Flatcar ]   [ No, go back ]
         ↑ FocusedButton       ↑ BlurredButton
```

The `Negative` button is selected by default (safer default — requires deliberate
left-arrow to select Yes). Set this explicitly with a custom accessor that
defaults to `false`.

**Enhancement — Danger styling on the confirm group:**

Override the group title style for the review step to use red:

```go
func (m *Model) buildReviewForm() *huh.Form {
    dangerTheme := huh.ThemeDracula()
    dangerTheme.Focused.Title = dangerTheme.Focused.Title.
        Foreground(lipgloss.Color("#ff5555")).  // Dracula red
        Bold(true)

    return huh.NewForm(
        huh.NewGroup(
            huh.NewConfirm().
                Title("⚠  DESTRUCTIVE — Install Flatcar to " + cfg.Disk.DevPath + "?").
                Description(m.reviewSummary()).
                Affirmative("Yes, wipe disk and install").
                Negative("No, go back").
                Value(&m.Wizard.State.Confirmed),
        ).Title("Point of No Return"),
    ).WithTheme(dangerTheme)
}
```

**Additional signal:** Print the disk in a bordered box in the description:

```
  ┌──────────────────────────────────────┐
  │  /dev/disk/by-id/ata-Samsung_860_EVO │
  │  500 GB · SATA · not removable       │
  │  ALL DATA WILL BE PERMANENTLY ERASED │
  └──────────────────────────────────────┘
```

---

### 2.4  Install Progress — The Package-Manager Pattern

**The canonical bubbletea pattern** for multi-phase long-running operations is
in `bubbletea/examples/package-manager`. The key mechanics:

1. Each phase returns a `tea.Cmd` that does the work and returns a typed `Msg`.
2. When a phase completes, the parent model calls `tea.Printf()` to print the
   completed line to the scroll buffer above the TUI.
3. The current phase is shown with: `spinner.View() + " " + phaseLabel + "  " + bar.View()`
4. The next phase is triggered by returning another `tea.Cmd`.

**For knuckle's install screen:**

```
✓ Generating Ignition config                           [done, in scroll buffer]
✓ Compiling Butane → Ignition JSON  [2ms]              [done, in scroll buffer]
✓ Downloading Flatcar 4152.1.0 (stable)  [1.2 GB]      [done, in scroll buffer]
⠙ Writing to /dev/disk/by-id/ata-Samsung...  ━━━━━━━━━░░░  78%   [active TUI]
```

**Implementation sketch:**

```go
type installPhase int

const (
    phaseGenerateIgnition installPhase = iota
    phaseCompileButane
    phaseDownload
    phaseWrite
    phaseVerify
    phaseDone
)

type phaseCompleteMsg struct {
    phase    installPhase
    duration time.Duration
    detail   string    // e.g. "1.2 GB downloaded"
}

type progressMsg float64  // 0.0–1.0 from install runner

// In Update():
case phaseCompleteMsg:
    label := phaseLabel(msg.phase)
    tea.Printf("✓ %s  [%s]\n", label, msg.detail)  // goes to scroll buffer
    m.currentPhase = msg.phase + 1
    if m.currentPhase < phaseDone {
        return m, m.startPhase(m.currentPhase)
    }

case progressMsg:
    return m, m.bar.SetPercent(float64(msg))  // animated bar update

case progress.FrameMsg:
    bar, cmd := m.bar.Update(msg)
    m.bar = bar.(progress.Model)
    return m, cmd

case spinner.TickMsg:
    spinner, cmd := m.spinner.Update(msg)
    m.spinner = spinner
    return m, cmd
```

**View:**
```go
func (m *Model) viewInstall() string {
    if m.currentPhase >= phaseDone {
        return ""  // Done view takes over
    }
    label := phaseLabel(m.currentPhase)
    return fmt.Sprintf(
        "\n  %s %s\n\n  %s\n",
        m.spinner.View(),
        label,
        m.bar.View(),
    )
}
```

The `tea.Printf` lines from completed phases scroll off the top naturally —
you get a live log above a stable progress widget, exactly like apt-get or
a Rust compilation.

---

### 2.5  Async Operations — GitHub Key Fetch Pattern

knuckle already has this right for the GitHub SSH key fetch:

```go
return func() tea.Msg {
    keys, err := github.FetchKeys(username)
    return fetchKeysMsg{keys: keys, err: err}
}
```

While fetching, show an inline spinner next to the field. The huh field stays
visible; the spinner appears below it via `viewWithForm()` adding:

```go
if m.fetching {
    b.WriteString(dimStyle.Render("  ⠙ Fetching keys from github.com/" + m.githubUserInput + "..."))
}
```

This is already in the code. The fix needed: use a real `spinner.Model` so the
frame actually animates (not a static `⣷`).

---

### 2.6  huh Group WithHide — Conditional Steps

huh supports hiding groups dynamically:

```go
huh.NewGroup(...).WithHideFunc(func() bool {
    return m.Wizard.State.Config.IgnitionURL != ""
})
```

When `IgnitionURL` is set, the Network/User/Sysext/Update groups get hidden
automatically — no manual `GoToStep()` needed.

**Current knuckle uses `GoToStep()` manually in `onFormComplete()`.**
Migrating to `WithHideFunc` is cleaner but requires the whole wizard to be a
single `huh.Form`, which is a bigger refactor. Not urgent for v1.

---

### 2.7  Form Validation — Move Into Fields

**Current pattern (problematic):**
```go
// In onFormComplete() — fires AFTER form completion
if err := validate.Channel(cfg.Channel); err != nil {
    m.err = err
    m.initForm()
    return m.activeForm.Init()
}
```

This resets the form on error, losing the user's input.

**Correct pattern — validate in the field:**
```go
huh.NewSelect[string]().
    Title("Release Channel").
    Options(channels...).
    Value(&m.Wizard.State.Config.Channel).
    Validate(func(s string) error {
        return validate.Channel(s)
    }),
```

huh will block the group advance and show the error inline in the group footer
(red text, from `Focused.ErrorMessage` style). No form reset needed.

For the Network step — hostname, CIDR, gateway, DNS should all have
`.Validate()` callbacks pointing into `internal/validate`:

```go
huh.NewInput().
    Title("IP Address (CIDR)").
    Placeholder("192.168.1.100/24").
    Value(&m.Wizard.State.Config.Network.Address).
    Validate(func(s string) error {
        if s == "" {
            return nil  // OK — empty means DHCP
        }
        return validate.CIDR(s)
    }),
```

---

## 3. Gap Analysis: Current knuckle vs Target

| Feature | Current State | Target State | Effort |
|---|---|---|---|
| **Step indicator** | Text breadcrumb `● Name ○ Name` | Styled rail, numbers, separator dots | Low |
| **Storage picker** | Manual cursor loop, raw text | `bubbles/table` with column headers | Medium |
| **Install progress bar** | Manual `strings.Repeat("█")` | `bubbles/progress` (spring animation) | Low |
| **Install spinner** | Static `⣷` character | `bubbles/spinner.Model` with `TickMsg` | Low |
| **Install log** | `m.ProgressMessages` accumulated in state | `tea.Printf` to scroll buffer | Low |
| **Field validation** | Post-completion in `onFormComplete()` | Per-field `.Validate()` callbacks | Medium |
| **Confirmation danger styling** | Default ThemeDracula confirm | Red title, bordered disk summary box | Low |
| **Spinner for async ops** | Static character, never animates | `spinner.Model` + `TickMsg` forwarding | Low |
| **Welcome header** | Shown on ALL form steps | Only on Welcome step | Low (1-line fix) |
| **Outer frame/title** | Plain text title | `lipgloss.RoundedBorder()` title card | Low |
| **GitHub fetch spinner** | Static char | Animated spinner.Model | Low |
| **huh theme** | ThemeDracula (good) | Keep ThemeDracula, extend for danger screen | Low |

---

## 4. Concrete Recommendations for knuckle (Priority Order)

### P0 — Fix Before Anything Else

**1. Welcome header shown on wrong steps.**
`viewWithForm()` always calls `viewWelcomeHeader()`. The system checks + channel
version table should only appear on `StepWelcome`. All other form steps should
show only the step rail + form.

```go
func (m *Model) viewWithForm() string {
    var b strings.Builder
    if m.Wizard.State.CurrentStep == model.StepWelcome {
        b.WriteString(m.viewWelcomeHeader())
    }
    b.WriteString(m.renderStepRail())
    // ...
```

**2. Move validation into huh fields.**
Field-level `.Validate()` callbacks on channel, hostname, CIDR, gateway. This
gives real-time feedback instead of post-completion resets.

---

### P1 — Core Professional Feel

**3. Add `bubbles/progress` + `bubbles/spinner` to the install step.**

Replace the manual `viewInstall()` bar with:

```go
import (
    "github.com/charmbracelet/bubbles/progress"
    "github.com/charmbracelet/bubbles/spinner"
)

// In Model:
installBar     progress.Model
installSpinner spinner.Model

// In New():
m.installBar = progress.New(
    progress.WithGradient("#50fa7b", "#ff79c6"),  // Dracula green → pink
    progress.WithWidth(40),
)
m.installSpinner = spinner.New()
m.installSpinner.Spinner = spinner.Dot
m.installSpinner.Style = lipgloss.NewStyle().Foreground(lipgloss.Color("#ff79c6"))
```

Forward `progress.FrameMsg` and `spinner.TickMsg` in `Update()`.

**4. Use `tea.Printf` for completed install phases** (instead of accumulating
in `m.Wizard.State.ProgressMessages`). This gives the real "watching apt-get"
feeling of output scrolling above a stable progress widget.

```go
case installProgressMsg:
    tea.Printf("  ✓ %s\n", string(msg))
    // Don't store in state — it's already in the terminal scroll buffer
```

**5. Start the install spinner on `StepInstall` init.**

```go
case model.StepInstall:
    return m, m.installSpinner.Tick  // begins the animation loop
```

---

### P2 — Storage Picker Polish

**6. Migrate Storage step to `bubbles/table`.**

Add `github.com/charmbracelet/bubbles/table` import. Build a table with:
- Column 0: Device path (short form: `/dev/vda` or last 24 chars of by-id path)
- Column 1: Model (truncated to 24 chars)
- Column 2: Size
- Column 3: Transport
- Column 4: `⚠ USB` or blank (removable flag)

The table handles ↑/↓ navigation internally. On Enter, read
`t.SelectedRow()[0]` to get the device path.

```go
// In viewStorage():
dangerNote := lipgloss.NewStyle().
    Bold(true).
    Foreground(lipgloss.Color("#ff5555")).
    Render("  ⚠  ALL DATA ON THE SELECTED DISK WILL BE ERASED")
return lipgloss.JoinVertical(lipgloss.Left,
    "  Select Target Disk\n",
    m.diskTable.View(),
    "",
    dangerNote,
)
```

---

### P3 — Confirmation Screen Enhancement

**7. Override theme for the destructive confirm.**

```go
func (m *Model) buildReviewForm() *huh.Form {
    dangerTheme := huh.ThemeDracula()
    dangerTheme.Focused.Title = lipgloss.NewStyle().
        Foreground(lipgloss.Color("#ff5555")).Bold(true)

    diskBox := lipgloss.NewStyle().
        Border(lipgloss.RoundedBorder()).
        BorderForeground(lipgloss.Color("#ff5555")).
        Padding(0, 2).
        Render(
            fmt.Sprintf("%s\n%s · %s\n⚠  ALL DATA WILL BE PERMANENTLY ERASED",
                cfg.Disk.DevPath, cfg.Disk.Model, cfg.Disk.SizeHuman),
        )

    return huh.NewForm(
        huh.NewGroup(
            huh.NewConfirm().
                Title("Install Flatcar Container Linux?").
                Description(diskBox + "\n\n" + m.reviewSummary()).
                Affirmative("Yes, wipe disk and install →").
                Negative("← Go back").
                Value(&m.Wizard.State.Confirmed),
        ),
    ).WithTheme(dangerTheme)
}
```

The `Negative` button is the default (cursor starts on it) because `Confirmed`
defaults to `false`. User must explicitly move right to Yes.

---

### P4 — Title Card

**8. Wrap the title in a rounded-border card.**

```go
var titleCard = lipgloss.NewStyle().
    Border(lipgloss.RoundedBorder()).
    BorderForeground(lipgloss.Color("#6272a4")).  // Dracula comment (subtle)
    Padding(0, 2).
    Bold(true).
    Foreground(lipgloss.Color("#bd93f9")).        // Dracula purple
    Render("🔧  Knuckle  ·  Flatcar Container Linux Installer")
```

This renders as:
```
╭──────────────────────────────────────────────────────╮
│  🔧  Knuckle  ·  Flatcar Container Linux Installer  │
╰──────────────────────────────────────────────────────╯
```

Render it once at the top of every `View()`. Does not need to re-render on
every tick (lipgloss is cheap but this is cosmetic).

---

## 5. What Makes It Feel Like a Professional Installer

The five things that separate "series of forms" from "professional installer":

1. **Consistent chrome** — same title card and step rail on every screen.
   The user always knows where they are and can never get lost.

2. **Animated progress during long ops** — the spring-animated `bubbles/progress`
   bar is the single biggest perceived quality improvement. Static bars feel
   broken; animated bars feel responsive.

3. **Scroll log during install** — `tea.Printf` for completed phases creates the
   "watching it work" feeling. Accumulating messages in a list view feels like
   a status dump; the scrolling log feels like live activity.

4. **Deliberate destructive confirmation** — the confirmation screen must make
   the user work slightly to proceed. Red title, bordered disk summary, and the
   cursor defaulting to "No" together create the right friction.

5. **Validation feedback in context** — huh's group footer error rendering shows
   errors *where the problem is*, not as a banner at the top of the screen.
   Users know which field to fix without searching.

---

## 6. Reference Implementations to Study

These exist in the bubbletea examples directory (clone to study):

| Repo | Example | Pattern |
|---|---|---|
| `charmbracelet/bubbletea` | `examples/package-manager` | Phase-driven progress with `tea.Printf` scroll log |
| `charmbracelet/bubbletea` | `examples/progress-download` | Real download with `IncrPercent` commands |
| `charmbracelet/huh` | `examples/burger` | Two-group form, per-field validation |
| `charmbracelet/huh` | `examples/dynamic-form` | `WithHideFunc` for conditional steps |

Clone URLs (read-only; do not push):
```
git clone https://github.com/charmbracelet/bubbletea /tmp/bubbletea-examples
git clone https://github.com/charmbracelet/huh /tmp/huh-examples
```

---

## 7. Appendix: Key Go Snippets

### Progress + Spinner model fields

```go
type Model struct {
    // ... existing fields ...

    // Install progress
    installBar     progress.Model
    installSpinner spinner.Model
    currentPhase   installPhase
}
```

### Init for install step

```go
func (m *Model) initInstallWidgets() {
    m.installBar = progress.New(
        progress.WithGradient("#50fa7b", "#ff79c6"),
        progress.WithWidth(m.width - 10),
    )
    m.installSpinner = spinner.New()
    m.installSpinner.Spinner = spinner.Dot
    m.installSpinner.Style = lipgloss.NewStyle().
        Foreground(lipgloss.Color("#ff79c6"))
}
```

### FrameMsg forwarding in Update

```go
case progress.FrameMsg:
    if m.Wizard.State.CurrentStep == model.StepInstall {
        bar, cmd := m.installBar.Update(msg)
        m.installBar = bar.(progress.Model)
        cmds = append(cmds, cmd)
    }
case spinner.TickMsg:
    if m.installing {
        spinner, cmd := m.installSpinner.Update(msg)
        m.installSpinner = spinner
        cmds = append(cmds, cmd)
    }
```

### Phase-complete handler

```go
case installProgressMsg:
    // Print to scroll buffer (above TUI)
    _ = tea.Printf("  ✓ %s\n", string(msg))
    // Update bar
    total := 5
    done := len(m.Wizard.State.ProgressMessages) + 1
    cmds = append(cmds, m.installBar.SetPercent(float64(done)/float64(total)))
```

### Install view

```go
func (m *Model) viewInstall() string {
    if !m.installing {
        return "\n  Press Enter to begin installation...\n"
    }
    return fmt.Sprintf(
        "\n  %s %s\n\n  %s\n\n  %s\n",
        m.installSpinner.View(),
        m.currentPhaseName(),
        m.installBar.View(),
        lipgloss.NewStyle().Foreground(lipgloss.Color("#6272a4")).
            Render("This may take several minutes. Do not power off."),
    )
}
```
