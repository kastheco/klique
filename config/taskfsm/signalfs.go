package taskfsm

import (
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"time"
)

const (
	StagingDir    = "staging"
	ProcessingDir = "processing"
	FailedDir     = "failed"
)

// EnsureSignalDirs creates baseDir and its staging, processing, and failed
// subdirectories with 0o755 permissions if they do not already exist.
func EnsureSignalDirs(baseDir string) error {
	for _, sub := range []string{"", StagingDir, ProcessingDir, FailedDir} {
		dir := filepath.Join(baseDir, sub)
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return fmt.Errorf("signalfs ensure dirs %s: %w", dir, err)
		}
	}
	return nil
}

// AtomicWrite writes body to baseDir/filename via a staging file followed by
// an atomic rename. If the rename fails, it attempts to remove the staging
// file as a best-effort cleanup.
func AtomicWrite(baseDir, filename, body string) error {
	if err := EnsureSignalDirs(baseDir); err != nil {
		return err
	}
	stagingPath := filepath.Join(baseDir, StagingDir, filename)
	if err := os.WriteFile(stagingPath, []byte(body), 0o644); err != nil {
		return fmt.Errorf("signalfs atomic write %s: %w", filename, err)
	}
	finalPath := filepath.Join(baseDir, filename)
	if err := os.Rename(stagingPath, finalPath); err != nil {
		_ = os.Remove(stagingPath)
		return fmt.Errorf("signalfs atomic write %s: %w", filename, err)
	}
	slog.Debug("signalfs: wrote signal", "file", filename, "dir", baseDir)
	return nil
}

// BeginProcessing atomically moves baseDir/filename into baseDir/processing/filename,
// returning the destination path. The caller owns the processing file until they call
// CompleteProcessing or FailProcessing.
func BeginProcessing(baseDir, filename string) (string, error) {
	src := filepath.Join(baseDir, filename)
	dst := filepath.Join(baseDir, ProcessingDir, filename)
	if err := os.Rename(src, dst); err != nil {
		return "", fmt.Errorf("signalfs begin processing %s: %w", filename, err)
	}
	slog.Debug("signalfs: began processing signal", "file", filename, "processing_path", dst)
	return dst, nil
}

// CompleteProcessing removes processingPath after the signal has been handled
// successfully. Failures are logged with slog.Warn but not returned — this
// mirrors the existing best-effort Consume* helpers.
func CompleteProcessing(processingPath string) {
	if err := os.Remove(processingPath); err != nil && !os.IsNotExist(err) {
		slog.Warn("signalfs: failed to remove processing file", "path", processingPath, "err", err)
		return
	}
	slog.Debug("signalfs: completed processing signal", "path", processingPath)
}

// FailProcessing moves baseDir/processing/filename to baseDir/failed/filename and
// writes a companion reason file at baseDir/failed/filename.reason. If the
// processing file is already gone, it logs and returns without panicking.
func FailProcessing(baseDir, filename, reason string) {
	src := filepath.Join(baseDir, ProcessingDir, filename)
	dst := filepath.Join(baseDir, FailedDir, filename)

	if err := os.Rename(src, dst); err != nil {
		if os.IsNotExist(err) {
			slog.Warn("signalfs: processing file already gone on fail", "file", filename)
			return
		}
		slog.Warn("signalfs: failed to move signal to failed dir", "file", filename, "err", err)
		return
	}

	reasonPath := dst + ".reason"
	content := time.Now().UTC().Format(time.RFC3339) + " " + reason + "\n"
	if err := os.WriteFile(reasonPath, []byte(content), 0o644); err != nil {
		slog.Warn("signalfs: failed to write reason file", "file", filename, "err", err)
	}
	slog.Info("signalfs: signal moved to failed", "file", filename, "reason", reason)
}

// RecoverInFlight reads baseDir/processing, skips directories, and moves each
// file back to baseDir/<name>. It returns the number of files successfully recovered.
// This should be called at startup to handle signals interrupted by a crash.
func RecoverInFlight(baseDir string) int {
	processingDir := filepath.Join(baseDir, ProcessingDir)
	entries, err := os.ReadDir(processingDir)
	if err != nil {
		if !os.IsNotExist(err) {
			slog.Warn("signalfs: failed to read processing dir for recovery", "dir", processingDir, "err", err)
		}
		return 0
	}

	recovered := 0
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		src := filepath.Join(processingDir, entry.Name())
		dst := filepath.Join(baseDir, entry.Name())
		// If a newer signal with the same name is already in the base directory
		// (written after this one was moved to processing/), skip the stale
		// in-flight copy and remove it so it does not overwrite the newer payload.
		if _, statErr := os.Lstat(dst); statErr == nil {
			slog.Warn("signalfs: skipping recovery, newer signal already in base dir",
				"file", entry.Name())
			_ = os.Remove(src)
			continue
		}
		if err := os.Rename(src, dst); err != nil {
			slog.Warn("signalfs: failed to recover in-flight signal", "file", entry.Name(), "err", err)
			continue
		}
		slog.Info("signalfs: recovered in-flight signal", "file", entry.Name())
		recovered++
	}
	return recovered
}
