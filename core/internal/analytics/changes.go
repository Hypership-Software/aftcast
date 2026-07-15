package analytics

import (
	"sort"

	"github.com/Hypership-Software/atlas/internal/schema"
)

type FileChange struct {
	Path         string
	LinesAdded   int
	LinesRemoved int
}

type ChangeSummary struct {
	Files        []FileChange
	LinesAdded   int
	LinesRemoved int
	Covered      bool
}

func (s ChangeSummary) Paths() []string {
	paths := make([]string, len(s.Files))
	for i, file := range s.Files {
		paths[i] = file.Path
	}
	sort.Strings(paths)
	return paths
}

func ObservedChanges(events []schema.TelemetryEvent) ChangeSummary {
	out := ChangeSummary{Covered: observationCaptured(events)}
	files := make(map[string]*FileChange)
	for _, call := range pairCalls(events) {
		if call.Pre.ToolClass != schema.ClassFileWrite || call.Post.ToolOK != schema.OutcomeOK {
			continue
		}
		if call.Pre.ChangeStats == nil {
			out.Covered = false
		}
		for _, path := range call.Pre.Files {
			if path == "" {
				continue
			}
			file := files[path]
			if file == nil {
				file = &FileChange{Path: path}
				files[path] = file
			}
			if call.Pre.ChangeStats != nil {
				file.LinesAdded += call.Pre.ChangeStats.LinesAdded
				file.LinesRemoved += call.Pre.ChangeStats.LinesRemoved
			}
		}
		if call.Pre.ChangeStats != nil {
			out.LinesAdded += call.Pre.ChangeStats.LinesAdded
			out.LinesRemoved += call.Pre.ChangeStats.LinesRemoved
		}
	}

	paths := make([]string, 0, len(files))
	for path := range files {
		paths = append(paths, path)
	}
	sort.Strings(paths)
	for _, path := range paths {
		out.Files = append(out.Files, *files[path])
	}
	return out
}

func observationCaptured(events []schema.TelemetryEvent) bool {
	for _, event := range events {
		if event.V >= schema.ObservationVersion {
			return true
		}
	}
	return false
}
