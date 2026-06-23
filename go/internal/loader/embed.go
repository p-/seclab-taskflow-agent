// SPDX-FileCopyrightText: GitHub, Inc.
// SPDX-License-Identifier: MIT

package loader

import "embed"

// bundledFS contains a snapshot of the Python package's bundled toolbox and
// personality YAML files, so that “seclab_taskflow_agent.*“ dotted paths
// resolve even when the Python source tree is not present. Regenerate with
// “make sync-embedded“.
//
//go:embed all:embedded
var bundledFS embed.FS
