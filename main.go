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

package main

import (
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"syscall"

	"github.com/dpeckett/immutableinit/internal/cmdline"
	"github.com/mitchellh/mapstructure"
)

const commandLinePrefix = "immutableinit"

type Options struct {
	Data       string   `cmdline:"data"`
	DataFSType string   `cmdline:"datafstype"`
	Mount      string   `cmdline:"mount"`
	Dirs       []string `cmdline:"dirs"`
	Cmd        string   `cmdline:"cmd"`
	Volatile   bool     `cmdline:"volatile"`
}

var DefaultOptions = Options{
	Mount: "/mnt/data",
	Dirs: []string{
		"/etc",
		"/home",
		"/root",
		"/srv",
		"/var",
	},
	Cmd: "/lib/systemd/systemd",
}

func main() {
	// Mount the /proc filesystem
	slog.Info("Mounting /proc")

	if err := syscall.Mount("proc", "/proc", "proc", 0, ""); err != nil {
		slog.Error("Failed to mount /proc", slog.Any("error", err))
		os.Exit(1)
	}

	// Read the command line
	slog.Info("Reading command line")

	cl := cmdline.NewCmdLine()
	if cl.Err != nil {
		slog.Error("Error reading /proc/cmdline", slog.Any("error", cl.Err))
		os.Exit(1)
	}

	// Parse the command line into a struct.
	opts := DefaultOptions
	decoder, err := mapstructure.NewDecoder(&mapstructure.DecoderConfig{
		Result:           &opts,
		TagName:          "cmdline",
		WeaklyTypedInput: true,
		DecodeHook: mapstructure.ComposeDecodeHookFunc(
			mapstructure.StringToSliceHookFunc(","),
			stringToBooleanHookFunc(),
		),
		MatchName: func(mapKey, fieldName string) bool {
			return strings.EqualFold(strings.TrimPrefix(strings.ReplaceAll(mapKey, "-", "_"), commandLinePrefix+"."), fieldName)
		},
	})
	if err != nil {
		slog.Error("Error creating decoder", slog.Any("error", err))
		os.Exit(1)
	}

	if err := decoder.Decode(cl.AsMap); err != nil {
		slog.Error("Error decoding command line", slog.Any("error", err))
		os.Exit(1)
	}

	// Mount the /tmp filesystem
	slog.Info("Mounting /tmp")

	if err := syscall.Mount("tmpfs", "/tmp", "tmpfs", 0, ""); err != nil {
		slog.Error("Failed to mount /tmp", slog.Any("error", err))
		os.Exit(1)
	}

	if opts.Volatile {
		slog.Info("Using volatile data mount")

		if err := syscall.Mount("tmpfs", opts.Mount, "tmpfs", 0, ""); err != nil {
			slog.Error("Failed to mount data mount", slog.Any("error", err))
			os.Exit(1)
		}
	} else {
		slog.Info("Using persistent data mount", slog.Any("device", opts.Data))

		if opts.Data == "" || opts.DataFSType == "" {
			slog.Error("data and data_fs_type must be specified")
			os.Exit(1)
		}

		if err := syscall.Mount(opts.Data, opts.Mount, opts.DataFSType, 0, ""); err != nil {
			slog.Error("Failed to mount data mount", slog.Any("error", err))
			os.Exit(1)
		}
	}

	for _, dir := range opts.Dirs {
		if _, err := os.Stat(dir); os.IsNotExist(err) {
			continue
		}

		slog.Info("Mounting overlay filesystem", slog.Any("dir", dir))

		// Create the upper and work directories
		upperDir := filepath.Join(opts.Mount, strings.TrimPrefix(dir, "/"))
		if err := os.MkdirAll(upperDir, 0o755); err != nil {
			slog.Error("Failed to create upperDir", slog.Any("dir", upperDir), slog.Any("error", err))
			os.Exit(1)
		}

		workDir := filepath.Join(opts.Mount, "."+strings.TrimPrefix(dir, "/")+"-work")
		if err := os.MkdirAll(workDir, 0o755); err != nil {
			slog.Error("Failed to create workDir", slog.Any("dir", workDir), slog.Any("error", err))
			os.Exit(1)
		}

		// Mount the overlay filesystem
		overlayOptions := "lowerdir=" + dir + ",workdir=" + workDir + ",upperdir=" + upperDir
		if err := syscall.Mount("overlay", dir, "overlay", 0, overlayOptions); err != nil {
			slog.Error("Failed to mount overlay filesystem", slog.Any("dir", dir), slog.Any("error", err))
			os.Exit(1)
		}
	}

	slog.Info("Executing init", slog.Any("cmd", opts.Cmd))

	argv := []string{opts.Cmd}
	argv = append(argv, os.Args[1:]...)

	if err := syscall.Exec(opts.Cmd, argv, os.Environ()); err != nil {
		slog.Error("Failed to exec init", slog.Any("cmd", opts.Cmd), slog.Any("error", err))
		os.Exit(1)
	}

	slog.Error("Init exited unexpectedly")
}

func stringToBooleanHookFunc() mapstructure.DecodeHookFunc {
	return func(f, t reflect.Type, data any) (interface{}, error) {
		if f.Kind() != reflect.String {
			return data, nil
		}

		if t != reflect.TypeOf(false) {
			return data, nil
		}

		switch strings.ToLower(data.(string)) {
		case "true", "yes", "1", "on":
			return true, nil
		case "false", "no", "0", "off":
			return false, nil
		}

		return false, fmt.Errorf("invalid boolean value: %q", data)
	}
}
