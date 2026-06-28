package main

import (
	"bufio"
	"bytes"
	"compress/gzip"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/filepicker"
	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// ponytail: single-file TUI, state machine, no unnecessary abstractions

// --- States ---
const (
	stateFilePicker = iota
	stateConverting
	stateMenu
	stateNaming
	statePlaying
	stateCompiling
	stateDone
)

// --- Types ---

type videoMeta struct {
	FPS    float64 `json:"fps"`
	Width  int     `json:"width"`
	Height int     `json:"height"`
	Frames int     `json:"frames"`
}

// --- Tea Messages ---

type metaMsg videoMeta
type frameMsg string
type convDoneMsg struct{}
type compileResultMsg struct {
	path string
	err  error
}
type tickMsg time.Time
type errMsg struct{ err error }

// --- Model ---

type model struct {
	state      int
	filepicker filepicker.Model
	spinner    spinner.Model
	textinput  textinput.Model

	videoPath    string
	frames       []string
	meta         videoMeta
	frameCh      chan interface{}
	framesLoaded int

	currentFrame int
	playing      bool
	standalone   bool // true when running a compiled standalone exe

	menuItems  []string
	menuCursor int

	resultPath string
	err        error

	termWidth  int
	termHeight int
}

// --- Styles ---

var (
	titleStyle   = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("205"))
	successStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("82"))
	accentStyle  = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("212"))
	dimStyle     = lipgloss.NewStyle().Foreground(lipgloss.Color("241"))
)

func newModel() model {
	fp := filepicker.New()
	fp.AllowedTypes = []string{".mp4", ".avi", ".mkv", ".mov", ".webm", ".flv", ".wmv"}
	fp.CurrentDirectory, _ = os.Getwd()
	fp.Height = 20

	sp := spinner.New()
	sp.Spinner = spinner.Dot
	sp.Style = lipgloss.NewStyle().Foreground(lipgloss.Color("205"))

	ti := textinput.New()
	ti.Placeholder = "my_ascii_video"
	ti.CharLimit = 64
	ti.Width = 40

	return model{
		state:      stateFilePicker,
		filepicker: fp,
		spinner:    sp,
		textinput:  ti,
		menuItems:  []string{"▶  Play Now", "💾 Save as Standalone .exe"},
		frameCh:    make(chan interface{}, 256),
	}
}

// --- Init ---

func (m model) Init() tea.Cmd {
	if m.state == statePlaying {
		return tickCmd(m.meta.FPS)
	}
	return tea.Batch(m.filepicker.Init(), m.spinner.Tick)
}

// --- Update ---

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.termWidth = msg.Width
		m.termHeight = msg.Height
	case tea.KeyMsg:
		if msg.String() == "ctrl+c" {
			return m, tea.Quit
		}
	}

	switch m.state {
	case stateFilePicker:
		return m.updateFilePicker(msg)
	case stateConverting:
		return m.updateConverting(msg)
	case stateMenu:
		return m.updateMenu(msg)
	case stateNaming:
		return m.updateNaming(msg)
	case statePlaying:
		return m.updatePlaying(msg)
	case stateCompiling:
		return m.updateCompiling(msg)
	case stateDone:
		return m.updateDone(msg)
	}
	return m, nil
}

// --- View ---

func (m model) View() string {
	switch m.state {
	case stateFilePicker:
		return m.viewFilePicker()
	case stateConverting:
		return m.viewConverting()
	case stateMenu:
		return m.viewMenu()
	case stateNaming:
		return m.viewNaming()
	case statePlaying:
		return m.viewPlaying()
	case stateCompiling:
		return m.viewCompiling()
	case stateDone:
		return m.viewDone()
	}
	return ""
}

// ============================================================
// State: File Picker
// ============================================================

func (m model) updateFilePicker(msg tea.Msg) (tea.Model, tea.Cmd) {
	if keyMsg, ok := msg.(tea.KeyMsg); ok {
		switch keyMsg.String() {
		case "esc", "q":
			return m, tea.Quit
		}
	}
	var cmd tea.Cmd
	m.filepicker, cmd = m.filepicker.Update(msg)
	if didSelect, path := m.filepicker.DidSelectFile(msg); didSelect {
		m.videoPath = path
		m.state = stateConverting
		m.framesLoaded = 0
		m.frames = nil
		m.frameCh = make(chan interface{}, 256)
		return m, tea.Batch(
			m.spinner.Tick,
			startConversion(m.videoPath, m.frameCh),
			waitForFrame(m.frameCh),
		)
	}
	return m, cmd
}

func (m model) viewFilePicker() string {
	title := titleStyle.Render("🎬 ASCII Zen — Select a video file")
	hint := dimStyle.Render("  ↑/↓ navigate • Enter select • q quit")
	return fmt.Sprintf("\n  %s\n\n%s\n\n%s\n", title, m.filepicker.View(), hint)
}

// ============================================================
// State: Converting
// ============================================================

func (m model) updateConverting(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case spinner.TickMsg:
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(msg)
		return m, cmd
	case metaMsg:
		m.meta = videoMeta(msg)
		return m, waitForFrame(m.frameCh)
	case frameMsg:
		m.frames = append(m.frames, string(msg))
		m.framesLoaded++
		return m, waitForFrame(m.frameCh)
	case convDoneMsg:
		m.state = stateMenu
		m.menuCursor = 0
		return m, nil
	case errMsg:
		m.err = msg.err
		return m, nil
	}
	return m, nil
}

func (m model) viewConverting() string {
	if m.err != nil {
		return fmt.Sprintf("\n  ✗ Error: %v\n\n  Press Ctrl+C to exit.\n", m.err)
	}
	pct := ""
	if m.meta.Frames > 0 {
		p := float64(m.framesLoaded) / float64(m.meta.Frames) * 100
		pct = fmt.Sprintf(" (%.0f%%)", p)
	}
	return fmt.Sprintf("\n  %s Converting video to ASCII...%s\n\n  Frames loaded: %d / %d\n\n  File: %s\n",
		m.spinner.View(), pct, m.framesLoaded, m.meta.Frames, filepath.Base(m.videoPath))
}

// ============================================================
// State: Menu
// ============================================================

func (m model) updateMenu(msg tea.Msg) (tea.Model, tea.Cmd) {
	if keyMsg, ok := msg.(tea.KeyMsg); ok {
		switch keyMsg.String() {
		case "up", "k":
			if m.menuCursor > 0 {
				m.menuCursor--
			}
		case "down", "j":
			if m.menuCursor < len(m.menuItems)-1 {
				m.menuCursor++
			}
		case "q":
			return m, tea.Quit
		case "esc":
			m.state = stateFilePicker
			m.frames = nil
			m.framesLoaded = 0
			return m, m.filepicker.Init()
		case "enter":
			switch m.menuCursor {
			case 0: // Play Now
				m.state = statePlaying
				m.currentFrame = 0
				m.playing = true
				return m, tickCmd(m.meta.FPS)
			case 1: // Save as .exe
				m.state = stateNaming
				m.textinput.Focus()
				base := filepath.Base(m.videoPath)
				name := strings.TrimSuffix(base, filepath.Ext(base))
				m.textinput.SetValue(name + "_ascii")
				return m, textinput.Blink
			}
		}
	}
	return m, nil
}

func (m model) viewMenu() string {
	title := successStyle.Render("✔ Conversion complete!")
	info := fmt.Sprintf("  %d frames loaded  •  %.0f FPS  •  %d×%d chars",
		len(m.frames), m.meta.FPS, m.meta.Width, m.meta.Height)

	var menu strings.Builder
	for i, item := range m.menuItems {
		cursor := "  "
		style := lipgloss.NewStyle()
		if i == m.menuCursor {
			cursor = "▸ "
			style = style.Bold(true).Foreground(lipgloss.Color("212"))
		}
		menu.WriteString(fmt.Sprintf("  %s%s\n", cursor, style.Render(item)))
	}

	hint := dimStyle.Render("  ↑/↓ navigate • Enter select • q quit")
	return fmt.Sprintf("\n  %s\n%s\n\n%s\n%s\n", title, info, menu.String(), hint)
}

// ============================================================
// State: Naming (text input for save filename)
// ============================================================

func (m model) updateNaming(msg tea.Msg) (tea.Model, tea.Cmd) {
	if keyMsg, ok := msg.(tea.KeyMsg); ok {
		switch keyMsg.String() {
		case "enter":
			name := strings.TrimSpace(m.textinput.Value())
			if name == "" {
				name = "ascii_video"
			}
			// Security: Prevent path traversal vulnerabilities
			name = filepath.Base(name)
			name = strings.ReplaceAll(name, string(filepath.Separator), "")
			if name == "." || name == ".." {
				name = "ascii_video"
			}
			
			m.state = stateCompiling
			return m, tea.Batch(
				m.spinner.Tick,
				compileStandalone(m.frames, m.meta, name),
			)
		case "esc":
			m.state = stateMenu
			return m, nil
		}
	}
	var cmd tea.Cmd
	m.textinput, cmd = m.textinput.Update(msg)
	return m, cmd
}

func (m model) viewNaming() string {
	title := accentStyle.Render("💾 Save as Standalone .exe")
	hint := dimStyle.Render("  Enter to build • Esc to go back")
	return fmt.Sprintf("\n  %s\n\n  Enter filename (without .exe):\n\n  %s\n\n%s\n", title, m.textinput.View(), hint)
}

// ============================================================
// State: Playing
// ============================================================

func (m model) updatePlaying(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case " ":
			m.playing = !m.playing
			if m.playing {
				return m, tickCmd(m.meta.FPS)
			}
			return m, nil
		case "left":
			jump := int(m.meta.FPS * 5)
			m.currentFrame -= jump
			if m.currentFrame < 0 {
				m.currentFrame = 0
			}
		case "right":
			jump := int(m.meta.FPS * 5)
			m.currentFrame += jump
			if m.currentFrame >= len(m.frames) {
				m.currentFrame = len(m.frames) - 1
			}
		case "esc":
			if !m.standalone {
				m.state = stateMenu
				m.playing = false
				return m, nil
			}
		case "q":
			return m, tea.Quit
		}
	case tickMsg:
		if !m.playing {
			return m, nil
		}
		m.currentFrame++
		if m.currentFrame >= len(m.frames) {
			m.currentFrame = 0 // loop
		}
		return m, tickCmd(m.meta.FPS)
	}
	return m, nil
}

func (m model) viewPlaying() string {
	if len(m.frames) == 0 {
		return "  No frames loaded."
	}

	frame := m.frames[m.currentFrame]
	w := m.meta.Width

	// Render frame with newlines
	var display strings.Builder
	for i := 0; i < len(frame); i += w {
		end := i + w
		if end > len(frame) {
			end = len(frame)
		}
		display.WriteString(frame[i:end])
		display.WriteByte('\n')
	}

	// Status
	status := "▶ Playing"
	if !m.playing {
		status = "⏸ Paused"
	}
	elapsed := time.Duration(float64(m.currentFrame) / m.meta.FPS * float64(time.Second))
	total := time.Duration(float64(len(m.frames)) / m.meta.FPS * float64(time.Second))

	// Progress bar
	barWidth := w
	if barWidth > m.termWidth-4 {
		barWidth = m.termWidth - 4
	}
	if barWidth < 10 {
		barWidth = 10
	}
	progress := float64(m.currentFrame) / float64(len(m.frames))
	filled := int(progress * float64(barWidth))
	if filled > barWidth {
		filled = barWidth
	}
	bar := strings.Repeat("█", filled) + strings.Repeat("░", barWidth-filled)

	statusLine := fmt.Sprintf("  %s   %s / %s", status, fmtDur(elapsed), fmtDur(total))

	controls := dimStyle.Render("  [Space] Play/Pause  [←/→] Seek ±5s  [q] Quit")
	if !m.standalone {
		controls = dimStyle.Render("  [Space] Play/Pause  [←/→] Seek ±5s  [Esc] Back  [q] Quit")
	}

	return fmt.Sprintf("%s\n%s\n  %s\n%s\n", display.String(), statusLine, bar, controls)
}

// ============================================================
// State: Compiling
// ============================================================

func (m model) updateCompiling(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case spinner.TickMsg:
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(msg)
		return m, cmd
	case compileResultMsg:
		if msg.err != nil {
			m.err = msg.err
		} else {
			m.resultPath = msg.path
		}
		m.state = stateDone
		return m, nil
	}
	return m, nil
}

func (m model) viewCompiling() string {
	return fmt.Sprintf("\n  %s Compiling standalone .exe...\n\n  This may take a moment.\n", m.spinner.View())
}

// ============================================================
// State: Done
// ============================================================

func (m model) updateDone(msg tea.Msg) (tea.Model, tea.Cmd) {
	if keyMsg, ok := msg.(tea.KeyMsg); ok {
		if keyMsg.String() == "q" || keyMsg.String() == "enter" {
			return m, tea.Quit
		}
	}
	return m, nil
}

func (m model) viewDone() string {
	if m.err != nil {
		return fmt.Sprintf("\n  ✗ Build failed: %v\n\n  Press q to exit.\n", m.err)
	}
	title := successStyle.Render("✔ Build successful!")
	return fmt.Sprintf("\n  %s\n\n  Saved to: %s\n\n  %s\n\n  Press q or Enter to exit.\n",
		title, m.resultPath,
		dimStyle.Render("This .exe runs on any Windows machine — no Python or Go needed."))
}

// ============================================================
// Commands
// ============================================================

// startConversion spawns the Python converter subprocess and streams
// metadata + frame data through ch.
func startConversion(videoPath string, ch chan<- interface{}) tea.Cmd {
	return func() tea.Msg {
		// Locate convert.py next to the executable
		exePath, err := os.Executable()
		if err != nil {
			close(ch)
			return errMsg{err}
		}
		convertScript := filepath.Join(filepath.Dir(exePath), "convert.py")

		// Fall back to CWD if not found next to exe
		if _, err := os.Stat(convertScript); err != nil {
			cwd, _ := os.Getwd()
			convertScript = filepath.Join(cwd, "convert.py")
		}

		pythonCmd := "python3"
		if _, err := exec.LookPath("python3"); err != nil {
			pythonCmd = "python"
		}
		cmd := exec.Command(pythonCmd, convertScript, "--video", videoPath, "--width", "120")
		stdout, err := cmd.StdoutPipe()
		if err != nil {
			close(ch)
			return errMsg{err}
		}
		cmd.Stderr = os.Stderr
		if err := cmd.Start(); err != nil {
			close(ch)
			return errMsg{err}
		}

		reader := bufio.NewReader(stdout)

		// 1. Read metadata line
		line, err := reader.ReadBytes('\n')
		if err != nil {
			close(ch)
			return errMsg{fmt.Errorf("reading metadata: %w", err)}
		}
		var meta videoMeta
		if err := json.Unmarshal(bytes.TrimSpace(line), &meta); err != nil {
			close(ch)
			return errMsg{fmt.Errorf("parsing metadata: %w", err)}
		}
		ch <- metaMsg(meta)

		// 2. Read fixed-size frames
		frameSize := meta.Width * meta.Height
		buf := make([]byte, frameSize)
		for {
			_, err := io.ReadFull(reader, buf)
			if err != nil {
				break
			}
			ch <- frameMsg(string(buf))
		}

		cmd.Wait()
		close(ch)
		return nil
	}
}

// waitForFrame reads one message from the channel and returns it as a tea.Msg.
func waitForFrame(ch <-chan interface{}) tea.Cmd {
	return func() tea.Msg {
		msg, ok := <-ch
		if !ok {
			return convDoneMsg{}
		}
		if msg == nil {
			return convDoneMsg{}
		}
		return msg.(tea.Msg)
	}
}

// compileStandalone writes embedded data files, builds a standalone exe, and cleans up.
func compileStandalone(frames []string, meta videoMeta, name string) tea.Cmd {
	return func() tea.Msg {
		// Find project directory (same as executable)
		exePath, err := os.Executable()
		if err != nil {
			return compileResultMsg{err: err}
		}
		projectDir := filepath.Dir(exePath)

		// Fall back to CWD if data.go isn't next to exe
		dataGoCheck := filepath.Join(projectDir, "data.go")
		if _, err := os.Stat(dataGoCheck); err != nil {
			projectDir, _ = os.Getwd()
		}

		// 1. Compress frames with gzip
		var raw bytes.Buffer
		for _, f := range frames {
			raw.WriteString(f)
		}
		var compressed bytes.Buffer
		gz := gzip.NewWriter(&compressed)
		if _, err := gz.Write(raw.Bytes()); err != nil {
			return compileResultMsg{err: fmt.Errorf("compressing: %w", err)}
		}
		gz.Close()

		// 2. Write embedded_data.gz
		dataGzPath := filepath.Join(projectDir, "embedded_data.gz")
		if err := os.WriteFile(dataGzPath, compressed.Bytes(), 0644); err != nil {
			return compileResultMsg{err: err}
		}
		defer os.Remove(dataGzPath)

		// 3. Write embedded_meta.json
		metaJSON, _ := json.Marshal(meta)
		metaJSONPath := filepath.Join(projectDir, "embedded_meta.json")
		if err := os.WriteFile(metaJSONPath, metaJSON, 0644); err != nil {
			return compileResultMsg{err: err}
		}
		defer os.Remove(metaJSONPath)

		// 4. Overwrite data.go with go:embed directives
		dataGoPath := filepath.Join(projectDir, "data.go")
		original, _ := os.ReadFile(dataGoPath)

		standaloneDataGo := `package main

import _ "embed"

//go:embed embedded_data.gz
var embeddedFrames []byte

//go:embed embedded_meta.json
var embeddedMeta []byte
`
		if err := os.WriteFile(dataGoPath, []byte(standaloneDataGo), 0644); err != nil {
			return compileResultMsg{err: err}
		}
		defer os.WriteFile(dataGoPath, original, 0644) // restore original

		// 5. Build standalone exe
		outputPath := filepath.Join(projectDir, name+".exe")
		buildCmd := exec.Command("go", "build", "-o", outputPath, ".")
		buildCmd.Dir = projectDir
		out, buildErr := buildCmd.CombinedOutput()
		if buildErr != nil {
			return compileResultMsg{err: fmt.Errorf("%s:\n%s", buildErr, string(out))}
		}

		return compileResultMsg{path: outputPath}
	}
}

// tickCmd returns a tea.Cmd that fires a tickMsg after one frame interval.
func tickCmd(fps float64) tea.Cmd {
	d := time.Duration(float64(time.Second) / fps)
	return tea.Tick(d, func(t time.Time) tea.Msg {
		return tickMsg(t)
	})
}

// --- Helpers ---

func fmtDur(d time.Duration) string {
	m := int(d.Minutes())
	s := int(d.Seconds()) % 60
	return fmt.Sprintf("%02d:%02d", m, s)
}

func loadEmbeddedFrames() ([]string, videoMeta, error) {
	gz, err := gzip.NewReader(bytes.NewReader(embeddedFrames))
	if err != nil {
		return nil, videoMeta{}, fmt.Errorf("decompressing: %w", err)
	}
	defer gz.Close()
	raw, err := io.ReadAll(gz)
	if err != nil {
		return nil, videoMeta{}, err
	}

	var meta videoMeta
	if err := json.Unmarshal(embeddedMeta, &meta); err != nil {
		return nil, videoMeta{}, fmt.Errorf("parsing metadata: %w", err)
	}

	frameSize := meta.Width * meta.Height
	var frames []string
	for i := 0; i < len(raw); i += frameSize {
		end := i + frameSize
		if end > len(raw) {
			break
		}
		frames = append(frames, string(raw[i:end]))
	}

	return frames, meta, nil
}

// --- Main ---

func main() {
	m := newModel()

	// Standalone mode: embedded data is present, skip straight to player
	if len(embeddedFrames) > 0 {
		frames, meta, err := loadEmbeddedFrames()
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error loading embedded data: %v\n", err)
			os.Exit(1)
		}
		m.frames = frames
		m.meta = meta
		m.state = statePlaying
		m.playing = true
		m.standalone = true
	}

	p := tea.NewProgram(m, tea.WithAltScreen())
	if _, err := p.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}
