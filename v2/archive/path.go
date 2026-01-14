package archive

import (
	"path"
	"strings"
)

// NormalizePath validates and normalizes a relative path for archive use.
func NormalizePath(p string) (string, error) {
	if p == "" {
		return "", ErrInvalidPath
	}
	if strings.Contains(p, "\x00") {
		return "", ErrInvalidPath
	}
	if strings.Contains(p, "\\") {
		return "", ErrInvalidPath
	}
	if strings.HasPrefix(p, "/") {
		return "", ErrInvalidPath
	}

	parts := strings.Split(p, "/")
	for i, part := range parts {
		switch part {
		case "..":
			return "", ErrInvalidPath
		case "":
			if i == len(parts)-1 {
				continue
			}
			if i == 0 {
				return "", ErrInvalidPath
			}
			return "", ErrInvalidPath
		}
	}

	clean := path.Clean(p)
	if clean == "." {
		return "", ErrInvalidPath
	}
	if strings.HasPrefix(clean, "../") || clean == ".." {
		return "", ErrInvalidPath
	}
	if strings.HasPrefix(clean, "/") {
		return "", ErrInvalidPath
	}

	return clean, nil
}

func normalizePath(p string) (string, error) {
	return NormalizePath(p)
}
