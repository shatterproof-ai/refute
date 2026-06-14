package edit

import "sort"

func orderedFileEdits(fileEdits []FileEdit) []FileEdit {
	ordered := make([]FileEdit, len(fileEdits))
	for i, fe := range fileEdits {
		ordered[i] = FileEdit{
			Path:  fe.Path,
			Edits: append([]TextEdit(nil), fe.Edits...),
		}
		sort.SliceStable(ordered[i].Edits, func(a, b int) bool {
			return textEditLess(ordered[i].Edits[a], ordered[i].Edits[b])
		})
	}

	sort.SliceStable(ordered, func(i, j int) bool {
		return ordered[i].Path < ordered[j].Path
	})
	return ordered
}

func textEditLess(a, b TextEdit) bool {
	if a.Range.Start.Line != b.Range.Start.Line {
		return a.Range.Start.Line < b.Range.Start.Line
	}
	if a.Range.Start.Character != b.Range.Start.Character {
		return a.Range.Start.Character < b.Range.Start.Character
	}
	if a.Range.End.Line != b.Range.End.Line {
		return a.Range.End.Line < b.Range.End.Line
	}
	if a.Range.End.Character != b.Range.End.Character {
		return a.Range.End.Character < b.Range.End.Character
	}
	return false
}
