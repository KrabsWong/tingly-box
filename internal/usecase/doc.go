// Package usecase holds application logic shared across every caller surface
// (CLI, TUI, Web handler, future GUI) for a given domain — Rule, Provider,
// Agent. Each domain is one concrete struct with Request-in/Result-out
// methods and typed errors; no user I/O happens in this package.
//
// See .design/usecase-layer.md for the full contract and rationale.
package usecase
