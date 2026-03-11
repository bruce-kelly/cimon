package views

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseDiffFiles_SingleFile(t *testing.T) {
	raw := "diff --git a/hello.go b/hello.go\nindex 1234567..abcdef0 100644\n--- a/hello.go\n+++ b/hello.go\n@@ -1,3 +1,5 @@\n package main\n+import \"fmt\"\n+\n func main() {\n+\tfmt.Println(\"hello\")\n }\n"
	files := ParseDiffFiles(raw)
	require.Len(t, files, 1)
	assert.Equal(t, "hello.go", files[0].Path)
	assert.Equal(t, 3, files[0].Additions)
	assert.Equal(t, 0, files[0].Deletions)
	assert.Equal(t, 0, files[0].Offset)
}

func TestParseDiffFiles_MultipleFiles(t *testing.T) {
	raw := "diff --git a/foo.go b/foo.go\n--- a/foo.go\n+++ b/foo.go\n@@ -1,3 +1,4 @@\n package foo\n+func Foo() {}\n // end\ndiff --git a/bar.go b/bar.go\n--- a/bar.go\n+++ b/bar.go\n@@ -1,4 +1,3 @@\n package bar\n-func Old() {}\n+func New() {}\n-// removed\n"
	files := ParseDiffFiles(raw)
	require.Len(t, files, 2)
	assert.Equal(t, "foo.go", files[0].Path)
	assert.Equal(t, 1, files[0].Additions)
	assert.Equal(t, 0, files[0].Deletions)
	assert.Equal(t, "bar.go", files[1].Path)
	assert.Equal(t, 1, files[1].Additions)
	assert.Equal(t, 2, files[1].Deletions)
	assert.Greater(t, files[1].Offset, 0)
}

func TestParseDiffFiles_EmptyDiff(t *testing.T) {
	files := ParseDiffFiles("")
	assert.Empty(t, files)
}

func TestParseDiffFiles_BinaryFile(t *testing.T) {
	raw := "diff --git a/image.png b/image.png\nBinary files /dev/null and b/image.png differ\n"
	files := ParseDiffFiles(raw)
	require.Len(t, files, 1)
	assert.Equal(t, "image.png", files[0].Path)
	assert.Equal(t, 0, files[0].Additions)
	assert.Equal(t, 0, files[0].Deletions)
}

func TestParseDiffFiles_Rename(t *testing.T) {
	raw := "diff --git a/old.go b/new.go\nsimilarity index 100%\nrename from old.go\nrename to new.go\n"
	files := ParseDiffFiles(raw)
	require.Len(t, files, 1)
	assert.Equal(t, "new.go", files[0].Path)
}

func TestParseDiffFiles_PathContainingB(t *testing.T) {
	raw := "diff --git a/internal/b/foo.go b/internal/b/foo.go\n--- a/internal/b/foo.go\n+++ b/internal/b/foo.go\n@@ -1,3 +1,4 @@\n package b\n+func Bar() {}\n"
	files := ParseDiffFiles(raw)
	require.Len(t, files, 1)
	assert.Equal(t, "internal/b/foo.go", files[0].Path)
}
