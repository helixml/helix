package main

import (
	"strconv"
	"strings"
	"unicode"
)

func getQueryParamString(name string, params map[string][]string, validValues []string) string {
	values, ok := params[name]
	if !ok || len(values) == 0 {
		return ""
	}

	for _, value := range values {
		for _, validValue := range validValues {
			if value == validValue {
				return value
			}
		}
	}

	return ""
}

func getQueryParamStringAny(name string, params map[string][]string) string {
	values, ok := params[name]
	if !ok || len(values) == 0 {
		return ""
	}

	for _, value := range values {
		if value != "" {
			return value
		}
	}

	return ""
}

func getQueryParamInteger(name string, params map[string][]string) int {
	if val, ok := params[name]; ok && len(val[0]) > 0 {
		parsedValue, err := strconv.Atoi(val[0])
		if err == nil {
			return parsedValue
		}
	}
	return 0
}

func doesQueryMatchString(value string, query string) bool {
	if query == "" {
		return true
	}
	value = strings.ToLower(removeNonAlphanumeric(value))
	query = strings.ToLower(removeNonAlphanumeric(query))
	return strings.Contains(value, query)
}

func removeNonAlphanumeric(s string) string {
	var sb strings.Builder
	for _, r := range s {
		if unicode.IsLetter(r) || unicode.IsNumber(r) {
			sb.WriteRune(r)
		}
	}
	return sb.String()
}
