package install

import (
	"fmt"
)

const (
	MaxDownloadSize  int64 = 1 << 30
	MaxExtractedSize int64 = 4 << 30
	MaxFileSize      int64 = 1 << 30
	MaxArchiveFiles        = 100_000
	MaxArchiveDepth        = 64
	MaxArchivePath         = 4096
)

type Request struct {
	Version  string
	Filename string
	URL      string
	SHA256   string
	Size     int64
}

// Progress describes the latest observable state of an installation.
// BytesReceived and BytesTotal are meaningful during StageDownload; the
// latter is zero when the archive size is unknown.
type Progress struct {
	Version       string
	Stage         Stage
	BytesReceived int64
	BytesTotal    int64
}

// ProgressReporter observes installation progress. Implementations must
// return promptly and must not affect the installation result.
type ProgressReporter interface {
	Report(Progress)
}

// ProgressReporterFunc adapts a function into a ProgressReporter.
type ProgressReporterFunc func(Progress)

func (f ProgressReporterFunc) Report(progress Progress) {
	if f != nil {
		f(progress)
	}
}

type WarningKind int

const (
	WarningUnknown WarningKind = iota
	WarningIntegrityUnavailable
	WarningCleanup
)

type Warning struct {
	Kind WarningKind
	Err  error
}

func (w Warning) Error() string {
	switch w.Kind {
	case WarningIntegrityUnavailable:
		return "archive checksum unavailable"
	case WarningCleanup:
		if w.Err != nil {
			return fmt.Sprintf("cleanup: %v", w.Err)
		}
		return "cleanup incomplete"
	default:
		if w.Err != nil {
			return w.Err.Error()
		}
		return "installation warning"
	}
}

type Result struct {
	Version  string
	Path     string
	Warnings []Warning
}

type Stage int

const (
	StageUnknown Stage = iota
	StageValidate
	StageLock
	StagePrepare
	StageDownload
	StageIntegrity
	StageExtract
	StageVerify
	StageCommit
	StageCleanup
)

func (s Stage) String() string {
	switch s {
	case StageValidate:
		return "validation"
	case StageLock:
		return "lock"
	case StagePrepare:
		return "preparation"
	case StageDownload:
		return "download"
	case StageIntegrity:
		return "integrity verification"
	case StageExtract:
		return "extraction"
	case StageVerify:
		return "toolchain verification"
	case StageCommit:
		return "commit"
	case StageCleanup:
		return "cleanup"
	default:
		return "installation"
	}
}

type Error struct {
	Stage        Stage
	Err          error
	RecoveryPath string
}

func (e *Error) Error() string {
	if e == nil {
		return "<nil>"
	}
	message := fmt.Sprintf("%s: %v", e.Stage, e.Err)
	if e.RecoveryPath != "" {
		message += fmt.Sprintf("; recovery backup preserved at %s", e.RecoveryPath)
	}
	return message
}

func (e *Error) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Err
}
