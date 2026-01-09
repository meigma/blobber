package cli

import (
	"fmt"
	"os"
	"sync"

	"github.com/charmbracelet/bubbles/progress"
	"github.com/spf13/viper"
	"golang.org/x/term"

	"github.com/meigma/blobber"
)

// progressMode returns the configured progress mode: "auto", "tty", or "plain".
func progressMode() string {
	mode := viper.GetString("progress")
	switch mode {
	case "auto", "tty", "plain":
		return mode
	default:
		return "auto"
	}
}

// shouldShowProgress returns true if progress bars should be displayed.
func shouldShowProgress() bool {
	mode := progressMode()

	// Plain mode disables progress
	if mode == "plain" {
		return false
	}

	// TTY mode forces progress regardless of terminal detection
	if mode == "tty" {
		return true
	}

	// Auto mode: show progress only if connected to a TTY
	return term.IsTerminal(int(os.Stderr.Fd()))
}

// charmProgress wraps the charmbracelet progress bar for byte-based operations.
type charmProgress struct {
	bar         progress.Model
	description string
	total       int64
}

// newCharmProgress creates a new charmbracelet progress bar.
func newCharmProgress(total int64, description string) *charmProgress {
	bar := progress.New(
		progress.WithDefaultGradient(),
		progress.WithWidth(40),
		progress.WithoutPercentage(),
	)

	return &charmProgress{
		bar:         bar,
		description: description,
		total:       total,
	}
}

// render outputs the progress bar to stderr.
func (p *charmProgress) render(transferred int64) {
	var percent float64
	if p.total > 0 {
		percent = float64(transferred) / float64(p.total)
	}

	// Format bytes transferred and total
	transferredStr := formatBytes(transferred)
	totalStr := formatBytes(p.total)

	// Clear the line and render the progress bar
	fmt.Fprintf(os.Stderr, "\r\033[K%s %s %s/%s",
		p.description,
		p.bar.ViewAs(percent),
		transferredStr,
		totalStr,
	)
}

// finish completes the progress bar display.
func (p *charmProgress) finish() {
	fmt.Fprintln(os.Stderr)
}

// formatBytes formats bytes in a human-readable format.
func formatBytes(b int64) string {
	const unit = 1024
	if b < unit {
		return fmt.Sprintf("%d B", b)
	}
	div, exp := int64(unit), 0
	for n := b / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %ciB", float64(b)/float64(div), "KMGTPE"[exp])
}

// newPushProgress creates a progress callback for push operations.
// Returns the callback and a finish function to call when done.
// Returns nil callback if progress should not be shown.
func newPushProgress() (callback blobber.ProgressCallback, finish func()) {
	if !shouldShowProgress() {
		return nil, func() {}
	}

	var bar *charmProgress
	var once sync.Once

	callback = func(event blobber.ProgressEvent) {
		once.Do(func() {
			bar = newCharmProgress(event.TotalBytes, "Uploading")
		})
		if bar != nil {
			bar.render(event.BytesTransferred)
		}
	}

	finish = func() {
		if bar != nil {
			bar.finish()
		}
	}

	return callback, finish
}

// newPullProgress creates a progress callback for pull operations.
// Returns the callback and a finish function to call when done.
// Returns nil callback if progress should not be shown.
func newPullProgress() (callback blobber.ProgressCallback, finish func()) {
	if !shouldShowProgress() {
		return nil, func() {}
	}

	var bar *charmProgress
	var once sync.Once

	callback = func(event blobber.ProgressEvent) {
		once.Do(func() {
			bar = newCharmProgress(event.TotalBytes, "Downloading")
		})
		if bar != nil {
			bar.render(event.BytesTransferred)
		}
	}

	finish = func() {
		if bar != nil {
			bar.finish()
		}
	}

	return callback, finish
}
