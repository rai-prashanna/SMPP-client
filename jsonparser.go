package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"os"
	"strings"
	"unicode/utf16"
)

// InputPDU models the "input_pdu" object. We use pointer types for optional fields
// so we can detect when fields are missing.

func init() {
	// Default GSM 03.38 characters (common set). Source: GSM 03.38 tables (practical subset).
	def := "@£$¥èéùìòÇ\nØø\rÅåΔ_ΦΓΛΩΠΨΣΘΞÆæßÉ !\"#¤%&'()*+,-./0123456789:;<=>?¡" +
		"ABCDEFGHIJKLMNOPQRSTUVWXYZÄÖÑÜ§¿abcdefghijklmnopqrstuvwxyzäöñüà"
	gsmDefault = make(map[rune]bool, len(def))
	for _, r := range def {
		gsmDefault[r] = true
	}
	// Extended table (characters that require escape; each consumes two septets)
	ext := "^{}\\[~]|€" // common extended characters
	gsmExtended = make(map[rune]bool, len(ext))
	for _, r := range ext {
		gsmExtended[r] = true
	}
}

// parseFile accepts either a JSON array or newline-delimited JSON objects (JSONL).
func parseFile(path string) ([]TestCase, error) {
	// Read entire file (same behavior as original). For very large files,
	// consider streaming with os.Open and json.Decoder directly.
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	trimmed := bytes.TrimSpace(b)
	if len(trimmed) == 0 {
		return nil, fmt.Errorf("input file is empty")
	}

	// Remove optional UTF-8 BOM so prefix detection works correctly.
	trimmed = bytes.TrimPrefix(trimmed, []byte{0xEF, 0xBB, 0xBF})
	if len(trimmed) == 0 {
		return nil, fmt.Errorf("input file is empty")
	}

	// If the content starts with '[' treat it as a JSON array.
	if bytes.HasPrefix(trimmed, []byte{'['}) {
		var tests []TestCase
		if err := json.Unmarshal(trimmed, &tests); err != nil {
			return nil, fmt.Errorf("error unmarshalling JSON array: %w", err)
		}
		return tests, nil
	}

	// Fallback: decode one or more JSON objects (JSONL or concatenated JSON objects).
	dec := json.NewDecoder(bytes.NewReader(trimmed))
	var tests []TestCase
	for {
		var tc TestCase
		if err := dec.Decode(&tc); err != nil {
			if err == io.EOF {
				break
			}
			return nil, fmt.Errorf("error decoding JSON object: %w", err)
		}
		tests = append(tests, tc)
	}

	if len(tests) == 0 {
		return nil, fmt.Errorf("no test cases found in file")
	}
	return tests, nil
}

// gsm7SeptetCount returns number of septets required for string s under GSM 03.38:
// - default characters = 1 septet
// - extended characters = 2 septets (escape + char)
// returns error if a rune is not representable.
func gsm7SeptetCount(s string) (int, error) {
	count := 0
	for _, r := range s {
		if gsmDefault[r] {
			count++
		} else if gsmExtended[r] {
			// extended table characters are represented using ESC + char => 2 septets
			count += 2
		} else {
			return 0, fmt.Errorf("rune %U (%q) not representable in GSM 03.38", r, r)
		}
	}
	return count, nil
}

// ucs2ByteLength computes the number of bytes when the string is encoded as UTF-16 (big-endian).
// Each UTF-16 code unit is 2 bytes; a rune > 0xFFFF becomes a surrogate pair (2 code units, 4 bytes).
func ucs2ByteLength(s string) int {
	codeUnits := utf16.Encode([]rune(s))
	return len(codeUnits) * 2
}

// computeSegments calculates how many SMS segments the message will occupy given encoding and UDH.
// We use the conventional per-segment user data character limits:
// - GSM 7-bit: single=160, concatenated=153
// - UCS-2: single=70, concatenated=67
// For unknown encodings we fallback to octet-based sizes: single=140, concatenated=134.
func computeSegments(dataCoding int, udhPresent bool, messageLength int) int {
	var perSegment int
	switch dataCoding {
	case 0:
		if udhPresent {
			perSegment = 153
		} else {
			perSegment = 160
		}
	case 8:
		if udhPresent {
			perSegment = 67
		} else {
			perSegment = 70
		}
	default:
		if udhPresent {
			perSegment = 134
		} else {
			perSegment = 140
		}
	}
	if messageLength <= 0 {
		return 0
	}
	return int(math.Ceil(float64(messageLength) / float64(perSegment)))
}

// validateTestCase performs the validations and returns a ValidationResult.
// Behavior notes to match the sample expectations:
//   - For data_coding == 0 we treat sm_length as the septet count (number of GSM 7-bit characters,
//     counting extended characters as 2). Incompatible characters cause failure.
//   - For data_coding == 8 we compute sm_length as UTF-16 bytes (number of code units * 2).
//   - Otherwise we fallback to len([]byte(short_message)) (UTF-8 bytes).
func validateTestCase(index int, tc TestCase) ValidationResult {
	res := ValidationResult{
		Index: index,
		Valid: true,
	}

	// Required field checks
	if tc.InputPdu.DestinationAddr == nil || strings.TrimSpace(*tc.InputPdu.DestinationAddr) == "" {
		res.Valid = false
		res.Errors = append(res.Errors, "Missing required field: destination_addr")
		// Expected output in sample for this case is a failed delivery with that error.
		res.ExpectedOutputMatch = (tc.ExpectedOutput.Error != nil && strings.Contains(*tc.ExpectedOutput.Error, "Missing required field"))
		return res
	}

	// Short message presence: we allow empty messages but compute accordingly.
	shortMsg := ""
	if tc.InputPdu.ShortMessage != nil {
		shortMsg = *tc.InputPdu.ShortMessage
	}

	dataCoding := 0
	if tc.InputPdu.DataCoding != nil {
		dataCoding = *tc.InputPdu.DataCoding
	}

	// Compute expected "length" according to encoding rule that matches your examples.
	var computedLength int
	switch dataCoding {
	case 0:
		// GSM 7-bit: compute septet count; incompatible characters -> error
		sep, err := gsm7SeptetCount(shortMsg)
		if err != nil {
			res.Valid = false
			res.Errors = append(res.Errors, "Message contains characters incompatible with data_coding 0 (GSM 7-bit)")
			res.Errors = append(res.Errors, err.Error())
			// expected_output for such sample indicates failed delivery with that error.
			if tc.ExpectedOutput.Error != nil && strings.Contains(*tc.ExpectedOutput.Error, "incompatible") {
				res.ExpectedOutputMatch = true
			}
			return res
		}
		computedLength = sep
	case 8:
		// 16-bit encoding (UCS-2 / UTF-16BE): use code units * 2 bytes
		computedLength = ucs2ByteLength(shortMsg)
	default:
		// fallback: raw UTF-8 byte length
		computedLength = len([]byte(shortMsg))
	}

	res.ComputedSmLength = computedLength

	// Compare sm_length if present in input PDU
	if tc.InputPdu.SmLength != nil {
		if *tc.InputPdu.SmLength != computedLength {
			res.Valid = false
			// Match the error wording shown in your sample outputs
			msg := fmt.Sprintf("sm_length indicates %d bytes but actual short_message is %d bytes (truncated or malformed PDU)", *tc.InputPdu.SmLength, computedLength)
			res.Errors = append(res.Errors, msg)
			// If expected_output contains that error text, flag a match
			if tc.ExpectedOutput.Error != nil && strings.Contains(*tc.ExpectedOutput.Error, "sm_length indicates") {
				res.ExpectedOutputMatch = true
			}
			return res
		}
	}

	// Determine UDH presence
	udhPresent := tc.InputPdu.Udh != nil && strings.TrimSpace(*tc.InputPdu.Udh) != ""
	res.Segments = computeSegments(dataCoding, udhPresent, computedLength)
	if udhPresent {
		res.Note = "UDHI present, SMSC should concatenate segments"
	}

	// Compare certain expected_output fields (delivery_status and segments) where applicable.
	// This is a best-effort comparison; expected_output may contain message ID and other fields
	// that are unrelated to validation rules we compute here.
	match := true
	if tc.ExpectedOutput.DeliveryStatus != nil {
		exp := strings.ToLower(strings.TrimSpace(*tc.ExpectedOutput.DeliveryStatus))
		if exp == "accepted" && !res.Valid {
			match = false
		}
		if exp == "failed" && res.Valid {
			match = false
		}
	}
	if tc.ExpectedOutput.Segments != nil {
		if *tc.ExpectedOutput.Segments != res.Segments {
			match = false
			res.Mismatches = append(res.Mismatches, fmt.Sprintf("expected segments %d but computed %d", *tc.ExpectedOutput.Segments, res.Segments))
		}
	}
	res.ExpectedOutputMatch = match

	return res
}

//func main() {
//	filePath := flag.String("file", "test-case.jsonl", "path to JSON/JSONL file containing test cases")
//	flag.Parse()
//
//	_, err := parseFile(*filePath)
//	if err != nil {
//		fmt.Fprintf(os.Stderr, "error parsing file: %v\n", err)
//		os.Exit(1)
//	}
//
//}
