// SPDX-FileCopyrightText: GitHub, Inc.
// SPDX-License-Identifier: MIT

package envutil

import (
	"os"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestTmpEnvApplyRestore(t *testing.T) {
	t.Setenv("EXISTING", "old")
	_, unsetExists := os.LookupEnv("BRAND_NEW_VAR_XYZ")
	require.False(t, unsetExists)

	te, err := Apply(map[string]string{
		"EXISTING":          "new",
		"BRAND_NEW_VAR_XYZ": "value",
	}, nil)
	require.NoError(t, err)
	require.Equal(t, "new", os.Getenv("EXISTING"))
	require.Equal(t, "value", os.Getenv("BRAND_NEW_VAR_XYZ"))

	te.Restore()
	require.Equal(t, "old", os.Getenv("EXISTING"))
	_, ok := os.LookupEnv("BRAND_NEW_VAR_XYZ")
	require.False(t, ok)
}

func TestTmpEnvTemplateExpansion(t *testing.T) {
	t.Setenv("SOURCE", "expanded")
	te, err := Apply(map[string]string{"TARGET": "{{ env('SOURCE') }}"}, nil)
	require.NoError(t, err)
	defer te.Restore()
	require.Equal(t, "expanded", os.Getenv("TARGET"))
}

func TestFilteredEnviron(t *testing.T) {
	t.Setenv("SECRET_TO_HIDE", "shh")
	t.Setenv("KEEP_ME", "yes")
	t.Setenv(DenylistEnv, "SECRET_TO_HIDE")

	env := FilteredEnviron()
	var sawSecret, sawKeep bool
	for _, kv := range env {
		if kv == "SECRET_TO_HIDE=shh" {
			sawSecret = true
		}
		if kv == "KEEP_ME=yes" {
			sawKeep = true
		}
	}
	require.False(t, sawSecret, "denylisted var must be filtered out")
	require.True(t, sawKeep)
}
