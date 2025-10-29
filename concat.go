package main

// isConcatenatedDone returns true when all concatenated parts are present.
func isConcatenatedDone(parts []string, total uint8) bool {
	for _, part := range parts {
		if part != "" {
			total--
		}
	}
	return total == 0
}
