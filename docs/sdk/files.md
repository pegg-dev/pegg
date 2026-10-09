---
icon: lucide/file-code-2
---

# Files

The engine mounts a **sandboxed filesystem** at the workspace root. All file
operations use virtual paths relative to that root, `~` expands to the home
directory, and `.gitignore` / `.peggignore` rules are respected. Attempting to
escape the root returns `sdk.ErrOutOfBounds` (wrapped as `OutOfBoundsError`).

## Reading

`ReadFile` returns the content with line numbers and metadata:

```go
content, err := eng.ReadFile("src/main.go")
// Path: src/main.go | Hash: 9a2b... | Size: 1820 | Lines: 64
// ---
//      1: package main
// ...
```

A missing file returns `sdk.ErrNotFound`. Every read records a content hash used by
`EditFile`'s change detection.

## Writing

```go
out, err := eng.WriteFile("notes.md", []byte("# Notes\n"))
// Path: notes.md | Size: 9 | Hash: 8f3a...
// File written successfully.
```

Writes are atomic (temp file + rename), create parent directories, and preserve
existing file permissions.

## Editing

`EditFile` applies a text replacement. The file must have been read first, and its
hash must still match — an external change returns `sdk.ErrFileChanged` and requires
a fresh read.

```go
out, err := eng.EditFile("notes.md", "# Notes", "# Updated notes", false)
```

| Parameter | Description |
|---|---|
| `oldString` | Exact text to find (must be non-empty) |
| `newString` | Replacement |
| `replaceAll` | `true` replaces every occurrence; `false` replaces the first |

A read (or write) of the same path satisfies the read-before-edit requirement.

## Deleting

```go
err := eng.DeleteFile("notes.md")
```

## Listing and stat

```go
res, err := eng.ListFiles("src")
// res.Path       = "src"
// res.FileCount  = 12
// res.DirCount   = 2
// res.TotalSize  = 48520
for _, e := range res.Entries {
	fmt.Println(e.Name, e.Path, e.Size, e.IsDir)
}

info, err := eng.StatFile("src/main.go")
// info.Path, info.Name, info.Size, info.IsDir, info.ModTime
```

`ListFiles` requires a directory; a missing path returns `sdk.ErrNotFound`.

## Errors

| Sentinel | Meaning |
|---|---|
| `sdk.ErrOutOfBounds` | Path resolves outside the workspace |
| `sdk.ErrNotFound` | File or directory does not exist |
| `sdk.ErrReadRequired` | `EditFile` called without a prior read |
| `sdk.ErrFileChanged` | File changed on disk since the last read |
| `sdk.ErrNoMatch` | `oldString` not found |

## Example

```go
eng.WriteFile("hello.txt", []byte("hello world"))

got, _ := eng.ReadFile("hello.txt")
eng.EditFile("hello.txt", "hello", "goodbye", false)

list, _ := eng.ListFiles(".")
for _, e := range list.Entries {
	fmt.Println(e.Path)
}
```