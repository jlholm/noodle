package config

import (
	"fmt"
	"os/exec"
	"strings"
)

// ValidateBranchPattern checks a branch template before an order exists.
// The template must include {todo_id}. {slug} is the only other placeholder.
func ValidateBranchPattern(pattern string) error {
	pattern = strings.TrimSpace(pattern)
	if pattern == "" {
		return fmt.Errorf("branch pattern is empty")
	}
	sample, err := branchPatternSample(pattern)
	if err != nil {
		return err
	}
	return ValidateBranchRef(sample)
}

// ValidateBranchRef checks a rendered branch name before Git sees it.
func ValidateBranchRef(ref string) error {
	ref = strings.TrimSpace(ref)
	if ref == "" {
		return fmt.Errorf("branch ref is empty")
	}
	cmd := exec.Command("git", "check-ref-format", "--branch", ref)
	out, err := cmd.CombinedOutput()
	if err != nil {
		detail := strings.TrimSpace(string(out))
		if detail == "" {
			detail = err.Error()
		}
		return fmt.Errorf("branch ref %q is invalid: %s", ref, detail)
	}
	return nil
}

func branchPatternSample(pattern string) (string, error) {
	var rendered strings.Builder
	seenTodoID := false
	for i := 0; i < len(pattern); {
		if pattern[i] != '{' {
			rendered.WriteByte(pattern[i])
			i++
			continue
		}
		end := strings.IndexByte(pattern[i:], '}')
		if end < 0 {
			return "", fmt.Errorf("branch pattern has an unclosed placeholder")
		}
		name := pattern[i+1 : i+end]
		switch name {
		case "todo_id":
			seenTodoID = true
			rendered.WriteString("id")
		case "slug":
			rendered.WriteString("item")
		case "":
			return "", fmt.Errorf("branch pattern has an empty placeholder")
		default:
			if !placeholderName(name) {
				return "", fmt.Errorf("branch pattern has a malformed placeholder")
			}
			return "", fmt.Errorf("branch pattern contains unknown placeholder %q", "{"+name+"}")
		}
		i += end + 1
	}
	if !seenTodoID {
		return "", fmt.Errorf("branch pattern omits {todo_id}")
	}
	return rendered.String(), nil
}

func placeholderName(name string) bool {
	if name == "" {
		return false
	}
	for _, r := range name {
		switch {
		case r >= 'a' && r <= 'z':
		case r >= 'A' && r <= 'Z':
		case r >= '0' && r <= '9':
		case r == '_':
		default:
			return false
		}
	}
	return true
}
