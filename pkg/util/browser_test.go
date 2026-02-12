// Copyright 2026 Google LLC
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package util

import (
	"os"
	"runtime"
	"testing"
)

func TestIsHeadlessEnvironment(t *testing.T) {
	// Save and restore env vars
	origHeadless := os.Getenv("SCION_HEADLESS")
	origDisplay := os.Getenv("DISPLAY")
	origWayland := os.Getenv("WAYLAND_DISPLAY")
	t.Cleanup(func() {
		os.Setenv("SCION_HEADLESS", origHeadless)
		os.Setenv("DISPLAY", origDisplay)
		os.Setenv("WAYLAND_DISPLAY", origWayland)
	})

	t.Run("SCION_HEADLESS=1 forces headless", func(t *testing.T) {
		os.Setenv("SCION_HEADLESS", "1")
		os.Setenv("DISPLAY", ":0")
		os.Setenv("WAYLAND_DISPLAY", "wayland-0")
		if !IsHeadlessEnvironment() {
			t.Error("expected headless when SCION_HEADLESS=1")
		}
	})

	t.Run("SCION_HEADLESS=0 does not force headless", func(t *testing.T) {
		os.Setenv("SCION_HEADLESS", "0")
		os.Setenv("DISPLAY", ":0")
		os.Setenv("WAYLAND_DISPLAY", "")
		if runtime.GOOS == "darwin" {
			if IsHeadlessEnvironment() {
				t.Error("expected non-headless on macOS")
			}
		} else {
			// DISPLAY is set, so not headless
			if IsHeadlessEnvironment() {
				t.Error("expected non-headless when DISPLAY is set")
			}
		}
	})

	t.Run("no display vars on linux means headless", func(t *testing.T) {
		os.Setenv("SCION_HEADLESS", "")
		os.Setenv("DISPLAY", "")
		os.Setenv("WAYLAND_DISPLAY", "")
		if runtime.GOOS == "darwin" {
			if IsHeadlessEnvironment() {
				t.Error("macOS should never return headless (unless forced)")
			}
		} else {
			if !IsHeadlessEnvironment() {
				t.Error("expected headless when no display vars set on non-macOS")
			}
		}
	})

	t.Run("DISPLAY set means not headless", func(t *testing.T) {
		os.Setenv("SCION_HEADLESS", "")
		os.Setenv("DISPLAY", ":0")
		os.Setenv("WAYLAND_DISPLAY", "")
		if IsHeadlessEnvironment() {
			t.Error("expected non-headless when DISPLAY is set")
		}
	})

	t.Run("WAYLAND_DISPLAY set means not headless", func(t *testing.T) {
		os.Setenv("SCION_HEADLESS", "")
		os.Setenv("DISPLAY", "")
		os.Setenv("WAYLAND_DISPLAY", "wayland-0")
		if IsHeadlessEnvironment() {
			t.Error("expected non-headless when WAYLAND_DISPLAY is set")
		}
	})
}
