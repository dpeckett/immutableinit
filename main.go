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
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/immutos/matchstick/internal/cmdline"
	"github.com/immutos/matchstick/internal/kmsg"
	"github.com/immutos/matchstick/internal/util"
	"github.com/mitchellh/mapstructure"
	"github.com/spf13/pflag"
	"golang.org/x/sys/unix"
)

const optionsPrefix = "matchstick"

type Options struct {
	// Data is the device to which write operations will be redirected.
	Data string `cmdline:"data"`
	// DataFSType is the filesystem type of the data device.
	DataFSType string `cmdline:"datafstype"`
	// The mountpoint to be used for the data filesystem.
	Mount string `cmdline:"mount"`
	// Dirs is a list of directories to overlay on top of the data filesystem.
	Dirs []string `cmdline:"dirs"`
	// Cmd is the init process to be executed after the filesystem has been setup.
	Cmd string `cmdline:"cmd"`
	// Volatile specifies whether the data filesystem should be volatile.
	Volatile bool `cmdline:"volatile"`
}

func main() {
	handlerOpts := &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}

	// Log to the kernel log (if available).
	if f, err := os.OpenFile("/dev/kmsg", os.O_WRONLY, 0); err == nil {
		defer func() {
			_ = f.Sync()
			_ = f.Close()
		}()

		slog.SetDefault(slog.New(kmsg.NewKmsgHandler(f, handlerOpts)).WithGroup("matchstick"))
	} else {
		slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr, handlerOpts)))
	}

	// Are we running in a container?
	container := runningInContainer()

	var fs pflag.FlagSet
	fs.Init(os.Args[0], pflag.ContinueOnError)

	var opts Options
	fs.StringVar(&opts.Data, "data", "", "The device to which write operations will be redirected")
	fs.StringVar(&opts.DataFSType, "datafstype", "", "The filesystem type of the data device")
	fs.StringVar(&opts.Mount, "mount", "/mnt/data", "The mountpoint to be used for the data filesystem")
	fs.StringSliceVar(&opts.Dirs, "dirs", []string{"/etc", "/home", "/root", "/srv", "/var"},
		"A list of directories to overlay on top of the data filesystem")
	fs.StringVar(&opts.Cmd, "cmd", "/lib/systemd/systemd",
		"The init process to be executed after the filesystem has been setup")
	fs.BoolVar(&opts.Volatile, "volatile", false, "Whether the data filesystem should be volatile")

	if err := fs.Parse(os.Args[1:]); err != nil {
		slog.Error("Failed to parse command line", slog.Any("error", err))
		os.Exit(1)
	}

	if !container {
		// Mount the /proc filesystem (so that we can read the kernel command line).
		if _, err := os.Stat("/proc/cmdline"); os.IsNotExist(err) {
			slog.Info("Mounting /proc")

			if err := unix.Mount("proc", "/proc", "proc", 0, ""); err != nil {
				slog.Error("Failed to mount /proc", slog.Any("error", err))
				os.Exit(1)
			}
		}

		// Parse the kernel command line.
		cl := cmdline.NewCmdLine()
		if cl.Err != nil {
			slog.Error("Error reading /proc/cmdline", slog.Any("error", cl.Err))
			os.Exit(1)
		}

		decoder, err := mapstructure.NewDecoder(&mapstructure.DecoderConfig{
			Result:           &opts,
			TagName:          "cmdline",
			WeaklyTypedInput: true,
			DecodeHook: mapstructure.ComposeDecodeHookFunc(
				mapstructure.StringToSliceHookFunc(","),
				util.StringToBooleanHookFunc(),
			),
			MatchName: func(mapKey, fieldName string) bool {
				return strings.EqualFold(strings.TrimPrefix(strings.ReplaceAll(mapKey, "-", "_"), optionsPrefix+"."), fieldName)
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
	}

	// If we're running in a container, we should immediately pass control to the init process.
	if container {
		slog.Info("Running in a container, passing control to init", slog.Any("cmd", opts.Cmd))

		argv := []string{opts.Cmd}
		argv = append(argv, os.Args[1:]...)

		if err := unix.Exec(opts.Cmd, argv, os.Environ()); err != nil {
			slog.Error("Failed to exec init", slog.Any("cmd", opts.Cmd), slog.Any("error", err))
			os.Exit(1)
		}
	}

	// Mount the /tmp filesystem (if necessary).
	if f, err := os.Create("/tmp/.matchstick"); err == nil {
		_ = f.Close()
		_ = os.Remove(f.Name())
	} else {
		slog.Info("Mounting /tmp")

		if err := unix.Mount("tmpfs", "/tmp", "tmpfs", 0, ""); err != nil {
			slog.Error("Failed to mount /tmp", slog.Any("error", err))
			os.Exit(1)
		}
	}

	if opts.Volatile {
		slog.Info("Using volatile data mount")

		if err := unix.Mount("tmpfs", opts.Mount, "tmpfs", 0, ""); err != nil {
			slog.Error("Failed to mount data mount", slog.Any("error", err))
			os.Exit(1)
		}
	} else {
		slog.Info("Using persistent data mount", slog.Any("device", opts.Data))

		if opts.Data == "" || opts.DataFSType == "" {
			slog.Error("data and data_fs_type must be specified")
			os.Exit(1)
		}

		if err := unix.Mount(opts.Data, opts.Mount, opts.DataFSType, 0, ""); err != nil {
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
		if err := unix.Mount("overlay", dir, "overlay", 0, overlayOptions); err != nil {
			slog.Error("Failed to mount overlay filesystem", slog.Any("dir", dir), slog.Any("error", err))
			os.Exit(1)
		}
	}

	slog.Info("Executing init", slog.Any("cmd", opts.Cmd))

	argv := []string{opts.Cmd}
	argv = append(argv, os.Args[1:]...)

	if err := unix.Exec(opts.Cmd, argv, os.Environ()); err != nil {
		slog.Error("Failed to exec init", slog.Any("cmd", opts.Cmd), slog.Any("error", err))
		os.Exit(1)
	}
}

// runningInContainer returns true if the process is running in a container.
func runningInContainer() bool {
	cmd := exec.Command("/usr/bin/systemd-detect-virt", "--container")
	cmd.Stderr = os.Stderr

	out, err := cmd.Output()
	if err != nil {
		return false
	}

	return strings.TrimSpace(string(out)) != "none"
}
