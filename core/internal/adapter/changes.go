package adapter

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"

	"github.com/Hypership-Software/aftcast/internal/schema"
	"github.com/pmezard/go-difflib/difflib"
)

const (
	maxDiffBytes = 256 * 1024
	maxDiffLines = 2000
)

func changeStats(h ccHook) *schema.ChangeStats {
	switch h.ToolName {
	case "Edit":
		var in struct {
			FilePath string `json:"file_path"`
			Old      string `json:"old_string"`
			New      string `json:"new_string"`
		}
		if json.Unmarshal(h.ToolInput, &in) != nil || in.FilePath == "" {
			return nil
		}
		return lineChangeStats(in.Old, in.New)
	case "MultiEdit":
		var in struct {
			FilePath string `json:"file_path"`
			Edits    []struct {
				Old string `json:"old_string"`
				New string `json:"new_string"`
			} `json:"edits"`
		}
		if json.Unmarshal(h.ToolInput, &in) != nil || in.FilePath == "" || len(in.Edits) == 0 {
			return nil
		}
		total := &schema.ChangeStats{}
		for _, edit := range in.Edits {
			stats := lineChangeStats(edit.Old, edit.New)
			if stats == nil {
				return nil
			}
			total.LinesAdded += stats.LinesAdded
			total.LinesRemoved += stats.LinesRemoved
		}
		return total
	case "Write":
		var in struct {
			FilePath string `json:"file_path"`
			Content  string `json:"content"`
		}
		if json.Unmarshal(h.ToolInput, &in) != nil || in.FilePath == "" || len(in.Content) > maxDiffBytes {
			return nil
		}
		file := in.FilePath
		if !filepath.IsAbs(file) {
			file = filepath.Join(h.Cwd, file)
		}
		before, ok := readDiffFile(file)
		if !ok {
			return nil
		}
		return lineChangeStats(before, in.Content)
	default:
		return nil
	}
}

func readDiffFile(path string) (string, bool) {
	info, err := os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return "", true
		}
		return "", false
	}
	if info.IsDir() || info.Size() > maxDiffBytes {
		return "", false
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return "", false
	}
	return string(raw), true
}

func lineChangeStats(before, after string) *schema.ChangeStats {
	if len(before) > maxDiffBytes || len(after) > maxDiffBytes {
		return nil
	}
	oldLines := splitDiffLines(before)
	newLines := splitDiffLines(after)
	if len(oldLines) > maxDiffLines || len(newLines) > maxDiffLines {
		return nil
	}
	stats := &schema.ChangeStats{}
	for _, op := range difflib.NewMatcherWithJunk(oldLines, newLines, false, nil).GetOpCodes() {
		switch op.Tag {
		case 'r':
			stats.LinesRemoved += op.I2 - op.I1
			stats.LinesAdded += op.J2 - op.J1
		case 'd':
			stats.LinesRemoved += op.I2 - op.I1
		case 'i':
			stats.LinesAdded += op.J2 - op.J1
		}
	}
	return stats
}

func splitDiffLines(text string) []string {
	if text == "" {
		return nil
	}
	lines := strings.SplitAfter(text, "\n")
	if lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}
	return lines
}
