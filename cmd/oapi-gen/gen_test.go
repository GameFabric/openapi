package main

import (
	"flag"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var update = flag.Bool("update", false, "update the golden files of this test")

func TestGenerator_Generate(t *testing.T) {
	dir := t.TempDir()
	copyTestFile(t, dir)

	gen := NewGenerator("json", false)

	got, err := gen.Generate(dir)
	require.NoError(t, err)
	if *update {
		_ = os.WriteFile("testdata/not-all.go", got, 0o644)
	}

	want, err := os.ReadFile("testdata/not-all.go")
	require.NoError(t, err)
	assert.Equal(t, string(want), string(got))
}

func TestGenerator_GenerateWithAll(t *testing.T) {
	dir := t.TempDir()
	copyTestFile(t, dir)

	gen := NewGenerator("json", true)

	got, err := gen.Generate(dir)
	require.NoError(t, err)
	if *update {
		_ = os.WriteFile("testdata/all.go", got, 0o644)
	}

	want, err := os.ReadFile("testdata/all.go")
	require.NoError(t, err)
	assert.Equal(t, string(want), string(got))
}

func copyTestFile(t *testing.T, dir string) {
	t.Helper()

	b, err := os.ReadFile("testdata/test.go")
	require.NoError(t, err)

	path := filepath.Join(dir, "test.go")
	err = os.WriteFile(path, b, 0o600)
	require.NoError(t, err)
}
