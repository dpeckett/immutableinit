// SPDX-License-Identifier: AGPL-3.0-or-later
/*
 * Copyright (C) 2024 Damian Peckett <damian@pecke.tt>.
 *
 * This program is free software: you can redistribute it and/or modify
 * it under the terms of the GNU Affero General Public License as published by
 * the Free Software Foundation, either version 3 of the License, or
 * (at your option) any later version.
 *
 * This program is distributed in the hope that it will be useful,
 * but WITHOUT ANY WARRANTY; without even the implied warranty of
 * MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
 * GNU Affero General Public License for more details.
 *
 * You should have received a copy of the GNU Affero General Public License
 * along with this program. If not, see <https://www.gnu.org/licenses/>.
 */

package kmsg

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"strings"
)

var _ slog.Handler = (*KmsgHandler)(nil)

// KmsgHandler is a slog.Handler that writes log messages to the kernel log.
type KmsgHandler struct {
	f     *os.File
	level slog.Leveler
	group string
	attr  map[string]slog.Attr
}

func NewKmsgHandler(f *os.File, opts *slog.HandlerOptions) *KmsgHandler {
	return &KmsgHandler{
		f:     f,
		level: opts.Level,
		attr:  make(map[string]slog.Attr),
	}
}

func (kh *KmsgHandler) Enabled(_ context.Context, level slog.Level) bool {
	return level >= kh.level.Level()
}

func (kh *KmsgHandler) Handle(_ context.Context, r slog.Record) error {
	var sb strings.Builder
	if kh.group != "" {
		sb.WriteString(kh.group)
		sb.WriteString(": ")
	}

	sb.WriteString(r.Message)

	for _, attr := range kh.attr {
		fmt.Fprintf(&sb, " %s=%v", attr.Key, attr.Value)
	}

	r.Attrs(func(attr slog.Attr) bool {
		fmt.Fprintf(&sb, " %s=%v", attr.Key, attr.Value)
		return true
	})

	if err := kh.writeString(r.Level, sb.String()); err != nil {
		return err
	}

	return nil
}

func (kh *KmsgHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	newHandler := *kh
	newHandler.attr = make(map[string]slog.Attr, len(kh.attr))

	for key, attr := range kh.attr {
		newHandler.attr[key] = attr
	}

	return kh
}

func (kh *KmsgHandler) WithGroup(name string) slog.Handler {
	newHandler := *kh
	newHandler.attr = make(map[string]slog.Attr, len(kh.attr))

	for key, attr := range kh.attr {
		newHandler.attr[key] = attr
	}

	newHandler.group = name
	return &newHandler
}

func (kh *KmsgHandler) writeString(level slog.Level, msg string) error {
	_, err := kh.f.WriteString(fmt.Sprintf("<%d>%s", toKLogLevel(level), msg))
	if err != nil {
		return err
	}

	return nil
}

// KLogLevel represents the log levels for kernel logging.
type KLogLevel uintptr

// Log levels as defined by the kernel syslog
const (
	KLogEmergency KLogLevel = 0
	KLogAlert     KLogLevel = 1
	KLogCritical  KLogLevel = 2
	KLogError     KLogLevel = 3
	KLogWarning   KLogLevel = 4
	KLogNotice    KLogLevel = 5
	KLogInfo      KLogLevel = 6
	KLogDebug     KLogLevel = 7
)

func toKLogLevel(level slog.Level) KLogLevel {
	switch level {
	case slog.LevelDebug:
		return KLogDebug
	case slog.LevelInfo:
		return KLogInfo
	case slog.LevelWarn:
		return KLogWarning
	case slog.LevelError:
		return KLogError
	default:
		return KLogInfo
	}
}
