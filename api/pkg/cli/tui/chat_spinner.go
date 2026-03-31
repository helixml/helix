package tui

import (
	"fmt"
	"math/rand"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
)

var britishVerbs = []string{
	"Waiting for kettle",
	"Going for a walk",
	"Taking afternoon tea",
	"Raise your pinky finger",
	"Complaining about the weather",
	"Watching the football",
	"Going to the pub",
	"Going for lunch",
	"Sunday roast anyone?",
	"Fancy a cuppa?",
	"Queueing politely",
	"Having a biscuit",
	"Consulting the cricket scores",
	"Buttering a scone",
	"Minding the gap",
	"Checking the post",
	"Putting on a cardigan",
	"Having a proper brew",
}

var tips = []string{
	"Use ctrl+enter to interrupt the agent with a new message",
	"Use {prefix} | to split vertically and work on two tasks",
	"Use {prefix} c to open a new tab for another task",
	"Use /web to open this task in the browser",
}

// SpinnerFrames for the flower/asterisk animation
var spinnerFrames = []string{"✽", "✦", "✽", "✧"}

// Spinner renders the Claude Code-style "thinking" indicator.
type Spinner struct {
	verb      string
	startTime time.Time
	frame     int
	tokens    int
	tip       string
	prefix    string // actual tmux prefix for tip rendering
}

func NewSpinner(prefix string) *Spinner {
	tip := tips[rand.Intn(len(tips))]
	tip = strings.ReplaceAll(tip, "{prefix}", prefix)
	return &Spinner{
		verb:      britishVerbs[rand.Intn(len(britishVerbs))],
		startTime: time.Now(),
		frame:     0,
		tip:       tip,
		prefix:    prefix,
	}
}

func (s *Spinner) Tick() {
	s.frame = (s.frame + 1) % len(spinnerFrames)

	// Change verb every ~15 seconds
	elapsed := time.Since(s.startTime)
	verbIndex := int(elapsed.Seconds()/15) % len(britishVerbs)
	s.verb = britishVerbs[verbIndex]
}

func (s *Spinner) SetTokens(n int) {
	s.tokens = n
}

func (s *Spinner) View() string {
	elapsed := time.Since(s.startTime)

	// Format elapsed time
	var timeStr string
	if elapsed < time.Minute {
		timeStr = fmt.Sprintf("%ds", int(elapsed.Seconds()))
	} else {
		timeStr = fmt.Sprintf("%dm%02ds", int(elapsed.Minutes()), int(elapsed.Seconds())%60)
	}

	// Token count
	tokenStr := ""
	if s.tokens > 0 {
		if s.tokens >= 1000 {
			tokenStr = fmt.Sprintf(" · ↑ %.1fk tokens", float64(s.tokens)/1000)
		} else {
			tokenStr = fmt.Sprintf(" · ↑ %d tokens", s.tokens)
		}
	}

	// Spinner line
	spinChar := lipgloss.NewStyle().Foreground(colorPrimary).Render(spinnerFrames[s.frame])
	verbStyle := lipgloss.NewStyle().Foreground(colorText).Italic(true)
	metaStyle := styleDim

	line1 := fmt.Sprintf("%s %s (%s%s)",
		spinChar,
		verbStyle.Render(s.verb+"…"),
		metaStyle.Render(timeStr),
		metaStyle.Render(tokenStr),
	)

	// Tip line
	tipStyle := lipgloss.NewStyle().Foreground(colorDim)
	line2 := fmt.Sprintf("  ⎿  %s", tipStyle.Render("Tip: "+s.tip))

	return line1 + "\n" + line2
}
