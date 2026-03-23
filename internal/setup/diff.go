package setup

import (
	"strings"

	"github.com/pmezard/go-difflib/difflib"
)

func unifiedDiff(path string, before, after []byte) (string, error) {
	if string(before) == string(after) {
		return "", nil
	}

	text, err := difflib.GetUnifiedDiffString(difflib.UnifiedDiff{
		A:        difflib.SplitLines(string(before)),
		B:        difflib.SplitLines(string(after)),
		FromFile: path,
		ToFile:   path,
		Context:  3,
	})
	if err != nil {
		return "", err
	}
	return strings.TrimRight(text, "\n"), nil
}
