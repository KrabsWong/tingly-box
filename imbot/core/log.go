package core

import "github.com/sirupsen/logrus"

// log.go holds core-package internal logging through the application-wide
// logrus logger. This is for diagnostics emitted by core helpers themselves
// (e.g. the Segments/Text shadow migration warning) — not a replacement for
// the per-bot Logger interface in logger.go, which stays injectable for bots.
//
// Use the logrus package-level functions directly; they target the standard
// logger, the same stream the rest of tingly-box watches.

// warn logs a warning through the package-wide logrus logger.
func warn(format string, args ...interface{}) {
	logrus.Warnf(format, args...)
}
