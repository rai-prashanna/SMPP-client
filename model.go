package main

type InputPDU struct {
	CommandID            *string `json:"command_id,omitempty"`
	ServiceType          *string `json:"service_type,omitempty"`
	SourceAddrTON        *int    `json:"source_addr_ton,omitempty"`
	SourceAddrNPI        *int    `json:"source_addr_npi,omitempty"`
	SourceAddr           *string `json:"source_addr,omitempty"`
	DestAddrTON          *int    `json:"dest_addr_ton,omitempty"`
	DestAddrNPI          *int    `json:"dest_addr_npi,omitempty"`
	DestinationAddr      *string `json:"destination_addr,omitempty"`
	EsmClass             *int    `json:"esm_class,omitempty"`
	ProtocolID           *int    `json:"protocol_id,omitempty"`
	PriorityFlag         *int    `json:"priority_flag,omitempty"`
	RegisteredDelivery   *int    `json:"registered_delivery,omitempty"`
	ReplaceIfPresentFlag *int    `json:"replace_if_present_flag,omitempty"`
	DataCoding           *int    `json:"data_coding,omitempty"` // 0 == 7-bit, 8 == 16-bit (UCS-2/UTF-16BE)
	Encoding             *string `json:"encoding,omitempty"`    // "7-bit" or "16-bit" (informational)
	SmLength             *int    `json:"sm_length,omitempty"`
	ShortMessage         *string `json:"short_message,omitempty"`
}

// ExpectedOutput models the "expected_output_pdu" object in your data.
type ExpectedOutput struct {
	CommandID     *string `json:"command_id,omitempty"`
	CommandStatus *int    `json:"command_status,omitempty"`
	MessageID     *string `json:"message_id,omitempty"`
}

// TestCase ties an input PDU with its expected output.
type TestCase struct {
	TestCaseId     int            `json:"test_case_id"`
	InputPdu       InputPDU       `json:"input_pdu"`
	ExpectedOutput ExpectedOutput `json:"expected_output_pdu"`
}

//"test_case_id": 1,

// ValidationResult holds computed validation details for each test case.
type ValidationResult struct {
	Index               int      `json:"index"`
	Valid               bool     `json:"valid"`
	Errors              []string `json:"errors,omitempty"`
	ComputedSmLength    int      `json:"computed_sm_length"`
	Segments            int      `json:"segments"`
	Note                string   `json:"note,omitempty"`
	ExpectedOutputMatch bool     `json:"expected_output_match"`
	Mismatches          []string `json:"mismatches,omitempty"`
}

// We build GSM 03.38 default and extended character sets as rune->bool maps.
// This is a pragmatic subset based on the standard (sufficient for typical text).
var gsmDefault map[rune]bool
var gsmExtended map[rune]bool
