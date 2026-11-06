package catalog

import (
	"regexp"
	"strings"
)

var (
	productIDCharacters = regexp.MustCompile(`[^a-zA-Z0-9]+`)
	productIDPrefix     = regexp.MustCompile(`(?i)(3dsvr|[0-9]{2}id|t[0-9]8|r18|4k|fc2ppv|fc2|[^0-9]+)`)
)

type productID struct {
	normalized string
	compressed string
	prefix     string
	digits     string
}

func parseProductID(original string) (productID, bool) {
	value := original
	if len(value) >= 2 && value[1] == '_' {
		value = value[2:]
	}
	value = productIDCharacters.ReplaceAllString(value, "")
	matches := productIDPrefix.FindAllStringIndex(value, -1)
	groups := make([]string, 0, len(matches)*2+1)
	previous := 0
	for _, match := range matches {
		if match[0] > previous {
			groups = append(groups, value[previous:match[0]])
		}
		groups = append(groups, value[match[0]:match[1]])
		previous = match[1]
	}
	if previous < len(value) {
		groups = append(groups, value[previous:])
	}
	if len(groups) > 0 && isDigits(groups[0]) {
		groups = groups[1:]
	}
	if len(groups) > 0 && !isDigits(groups[len(groups)-1]) {
		groups = groups[:len(groups)-1]
	}
	digitIndex := -1
	for i, group := range groups {
		if isDigits(group) && (digitIndex < 0 || len(group) >= len(groups[digitIndex])) {
			digitIndex = i
		}
	}
	if digitIndex < 0 {
		return productID{}, false
	}
	alphaIndex := -1
	for i := 0; i < digitIndex; i++ {
		if !isDigits(groups[i]) && (alphaIndex < 0 || len(groups[i]) >= len(groups[alphaIndex])) {
			alphaIndex = i
		}
	}
	if alphaIndex < 0 {
		return productID{}, false
	}
	prefix := strings.ToUpper(strings.Join(groups[alphaIndex:digitIndex], ""))
	switch prefix {
	case "DGCEAD", "DGCEMD", "DGCESD":
		prefix = prefix[2:]
	case "FC2PPV":
		prefix = "FC2"
	}
	digits := groups[digitIndex]
	number := strings.TrimLeft(digits, "0")
	if number == "" {
		number = "0"
	}
	compressed := prefix + "-" + number
	if len(digits) != 2 {
		digits = padNumber(number, 3)
	}
	if prefix == "T" && len(digits) == 5 {
		prefix += digits[:2]
		digits = digits[2:]
	}
	return productID{normalized: prefix + "-" + digits, compressed: compressed, prefix: prefix, digits: digits}, true
}

func isDigits(value string) bool {
	if value == "" {
		return false
	}
	for _, char := range value {
		if char < '0' || char > '9' {
			return false
		}
	}
	return true
}

func padNumber(value string, size int) string {
	if len(value) >= size {
		return value
	}
	return strings.Repeat("0", size-len(value)) + value
}

func NormalizeProductID(value string) string {
	if id, ok := parseProductID(value); ok {
		return id.normalized
	}
	return value
}

func IsProductID(value string) bool {
	_, ok := parseProductID(value)
	return ok
}

func CompressProductID(value string) string {
	if id, ok := parseProductID(value); ok {
		return id.compressed
	}
	return value
}

func ProductIDVariations(value string) []string {
	id, ok := parseProductID(value)
	if !ok {
		return []string{value}
	}
	number := strings.TrimLeft(id.digits, "0")
	if number == "" {
		number = "0"
	}
	values := []string{
		id.normalized,
		id.prefix + "-" + padNumber(number, 4),
		strings.ToLower(id.prefix + padNumber(number, 3)),
		strings.ToLower(id.prefix + padNumber(number, 5)),
	}
	result := make([]string, 0, len(values))
	seen := make(map[string]bool, len(values))
	for _, variation := range values {
		if !seen[variation] {
			seen[variation] = true
			result = append(result, variation)
		}
	}
	return result
}
