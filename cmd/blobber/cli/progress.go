package cli

import (
	"os"
	"sync"

	"github.com/schollz/progressbar/v3"
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

// newProgressBar creates a new progress bar for byte-based operations.
func newProgressBar(total int64, description string) *progressbar.ProgressBar {
	return progressbar.NewOptions64(
		total,
		progressbar.OptionSetDescription(description),
		progressbar.OptionSetWriter(os.Stderr),
		progressbar.OptionShowBytes(true),
		progressbar.OptionSetWidth(40),
		progressbar.OptionShowCount(),
		progressbar.OptionSetRenderBlankState(true),
		progressbar.OptionUseANSICodes(true),
	)
}

// newPushProgress creates a progress callback for push operations.
// Returns the callback and a finish function to call when done.
// Returns nil callback if progress should not be shown.
func newPushProgress() (callback blobber.ProgressCallback, finish func()) {
	if !shouldShowProgress() {
		return nil, func() {}
	}

	var bar *progressbar.ProgressBar
	var once sync.Once

	callback = func(event blobber.ProgressEvent) {
		once.Do(func() {
			bar = newProgressBar(event.TotalBytes, "Uploading")
		})
		if bar != nil {
			//nolint:errcheck // progress bar errors are not critical
			bar.Set64(event.BytesTransferred)
		}
	}

	finish = func() {
		if bar != nil {
			//nolint:errcheck // progress bar errors are not critical
			bar.Finish()
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

	var bar *progressbar.ProgressBar
	var once sync.Once

	callback = func(event blobber.ProgressEvent) {
		once.Do(func() {
			bar = newProgressBar(event.TotalBytes, "Downloading")
		})
		if bar != nil {
			//nolint:errcheck // progress bar errors are not critical
			bar.Set64(event.BytesTransferred)
		}
	}

	finish = func() {
		if bar != nil {
			//nolint:errcheck // progress bar errors are not critical
			bar.Finish()
		}
	}

	return callback, finish
}
