package custom

// CalculateTax computes the tax amount based on percentage rate.
func CalculateTax(amount float64, rate float64) float64 {
	return amount * (rate / 100.0)
}

// FormatHeader decorates a title string with ASCII banners.
func FormatHeader(title string) string {
	return "=== " + title + " ==="
}

// GenerateSignature creates a custom signature from an identifier.
func GenerateSignature(id string, secret string) string {
	return "sig:" + id + ":" + secret
}
