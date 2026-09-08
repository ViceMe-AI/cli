package replicacontent

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
)

var ErrCreatorEntryBoundary = errors.New("Website Replica creator entry markers are incomplete or nested")

// stripCreatorEntry removes only explicitly delimited creator UI from the
// frozen delivery copy. The original worktree and all unmarked bytes survive.
func stripCreatorEntry(name string, data []byte) ([]byte, bool, error) {
	if !bytes.Contains(data, []byte("VICEME_CREATOR_ENTRY_")) {
		return data, false, nil
	}
	markers := map[string]string{
		"<!-- VICEME_CREATOR_ENTRY_BEGIN -->": "<!-- VICEME_CREATOR_ENTRY_END -->",
		"/* VICEME_CREATOR_ENTRY_BEGIN */":    "/* VICEME_CREATOR_ENTRY_END */",
		"{/* VICEME_CREATOR_ENTRY_BEGIN */}":  "{/* VICEME_CREATOR_ENTRY_END */}",
	}
	var result bytes.Buffer
	end := ""
	removed := false
	for _, line := range bytes.SplitAfter(data, []byte("\n")) {
		text := string(bytes.TrimSpace(line))
		if closing, ok := markers[text]; ok {
			if end != "" {
				return nil, false, fmt.Errorf("%w: %s", ErrCreatorEntryBoundary, name)
			}
			end, removed = closing, true
			continue
		}
		for _, closing := range markers {
			if text == closing {
				if end != closing {
					return nil, false, fmt.Errorf("%w: %s", ErrCreatorEntryBoundary, name)
				}
				end = ""
				goto nextLine
			}
		}
		if bytes.Contains(line, []byte("VICEME_CREATOR_ENTRY_")) {
			return nil, false, fmt.Errorf("%w: %s", ErrCreatorEntryBoundary, name)
		}
		if end == "" {
			result.Write(line)
		}
	nextLine:
	}
	if end != "" {
		return nil, false, fmt.Errorf("%w: %s", ErrCreatorEntryBoundary, name)
	}
	return result.Bytes(), removed, nil
}

// Existing ZIPs keep their exact bytes unless a managed creator entry is
// present. Validation precedes removal so a block cannot hide credentials.
func stripCreatorEntriesFromZIP(archive *FrozenSourceArchive) (_ []SourceArchiveExclusion, returnErr error) {
	input, err := os.Open(archive.filename)
	if err != nil {
		return nil, err
	}
	defer func() {
		if input != nil {
			returnErr = errors.Join(returnErr, input.Close())
		}
	}()
	info, err := input.Stat()
	if err != nil {
		return nil, err
	}
	plan, err := validatePublishArchive(input, info.Size())
	if err != nil {
		return nil, err
	}
	var files []frozenSourceFile
	excluded := make([]SourceArchiveExclusion, 0)
	for index, file := range plan.files {
		reader, err := file.entry.Open()
		if err != nil {
			return nil, err
		}
		data, readErr := io.ReadAll(io.LimitReader(reader, int64(MaxFileBytes)+1))
		if err := errors.Join(readErr, reader.Close()); err != nil {
			return nil, err
		}
		data, removed, err := stripCreatorEntry(file.name, data)
		if err != nil {
			return nil, err
		}
		if removed {
			excluded = append(excluded, SourceArchiveExclusion{Path: file.name, Reason: "creator-entry-blocks"})
		}
		snapshot := filepath.Join(archive.directory, fmt.Sprintf("entry-%06d.snapshot", index))
		if err := os.WriteFile(snapshot, data, 0o600); err != nil {
			return nil, err
		}
		files = append(files, frozenSourceFile{name: file.name, snapshot: snapshot, mode: file.mode, zipMethod: file.entry.Method, size: uint64(len(data))})
	}
	if len(excluded) > 0 {
		// Windows cannot replace an open file. Close the validated input before
		// replacing our private, not-yet-confirmed snapshot, never the user ZIP.
		if err := input.Close(); err != nil {
			return nil, err
		}
		input = nil
		if err := os.Remove(archive.filename); err != nil {
			return nil, err
		}
		sort.Slice(files, func(i, j int) bool { return files[i].name < files[j].name })
		if err := writeDeterministicSourceZIP(archive.filename, files); err != nil {
			return nil, err
		}
	}
	for _, file := range files {
		if err := os.Remove(file.snapshot); err != nil {
			return nil, err
		}
	}
	return excluded, nil
}
