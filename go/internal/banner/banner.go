// SPDX-FileCopyrightText: GitHub, Inc.
// SPDX-License-Identifier: MIT

// Package banner provides the CLI startup banner.
package banner

// Get returns the startup banner string shown when the agent runs.
func Get() string {
	return `
  ____            _          _     _____         _     __ _
 / ___|  ___  ___| |    __ _| |__ |_   _|_ _ ___| | __/ _| | _____      __
 \___ \ / _ \/ __| |   / _` + "`" + ` | '_ \  | |/ _` + "`" + ` / __| |/ / |_| |/ _ \ \ /\ / /
  ___) |  __/ (__| |__| (_| | |_) | | | (_| \__ \   <|  _| | (_) \ V  V /
 |____/ \___|\___|_____\__,_|_.__/  |_|\__,_|___/_|\_\_| |_|\___/ \_/\_/
        SecLab Taskflow Agent (Go) — AI workflows for security
`
}
