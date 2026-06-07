// Package utils Contains generic utilities that can be used in all packages
package utils

import (
	"maps"
	"slices"
)

// Deduplicate the given string array
func Deduplicate(strs []string) []string {
	strMap := map[string]bool{}
	for _, v := range strs {
		strMap[v] = true
	}
	return slices.Collect(maps.Keys(strMap))
}

// DeduplicateAndSort the given strings
func DeduplicateAndSort(strs []string) []string {
	result := Deduplicate(strs)
	slices.Sort(result)
	return result
}
