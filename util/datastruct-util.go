package util

// MapKeys - Extract the keys from a string-keyed map.
func MapKeys(fullMap map[string]string) []string {
	keys := make([]string, len(fullMap))
	i := 0
	for key := range fullMap {
		keys[i] = key
		i++
	}
	return keys
}
