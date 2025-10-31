package main

import (
	"bufio"
	"crypto/tls"
	_ "encoding/json"
	"fmt"
	"log"
	"net"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/fatih/color"
	_ "github.com/fatih/color"
	"github.com/joho/godotenv"
	"github.com/linxGnu/gosmpp"
	"github.com/linxGnu/gosmpp/data"
	"github.com/linxGnu/gosmpp/pdu"
)

var requestTracker = make(map[int32]*pdu.SubmitSM) // Track request based on sequence_number
var testCaseTracker = make(map[int32]*TestCase)    // Track PDU test case based on sequence_number

var testCases []TestCase // replace with actual type

func main() {

	var wg sync.WaitGroup

	wg.Add(1)
	go sendingAndReceiveSMS(&wg)

	wg.Wait()

}

func sendingAndReceiveSMS(wg *sync.WaitGroup) {
	testCases, parserError := parseFile("test-case.jsonl")

	if parserError != nil {
		color.Green("Rebinding but error:", parserError)
		//fmt.Fprintf(os.Stderr, "error parsing file: %v\n", parserError)
		os.Exit(1)
	}

	defer wg.Done()
	_ = godotenv.Load() // ignore error; .env is optional
	server := os.Getenv("SMPP_HOST") + ":" + os.Getenv("SMPP_PORT")
	systemId := os.Getenv("SYSTEM_ID")
	password := os.Getenv("PASSWORD")

	auth := gosmpp.Auth{
		SMSC:       server,
		SystemID:   systemId,
		Password:   password,
		SystemType: "",
	}
	var TLSDialer = func(addr string) (net.Conn, error) {
		conf := &tls.Config{
			InsecureSkipVerify: true,
		}
		return tls.Dial("tcp", addr, conf)
	}
	var trans *gosmpp.Session
	var err error
	trans, err = gosmpp.NewSession(
		gosmpp.TRXConnector(TLSDialer, auth),
		gosmpp.Settings{
			EnquireLink: 5 * time.Second,

			ReadTimeout: 10 * time.Second,

			OnSubmitError: func(_ pdu.PDU, err error) {
				log.Fatal("SubmitPDU error:", err)
			},

			OnReceivingError: func(err error) {
				color.Green("Receiving PDU/Network error:", err)
			},

			OnRebindingError: func(err error) {
				color.Green("Rebinding but error:", err)
			},

			OnPDU: handlePDU(&trans),

			OnClosed: func(state gosmpp.State) {
				color.Green("State :", state)
			},
		}, 5*time.Second)

	if err != nil {
		log.Fatal(err)
	}

	defer func() {
		_ = trans.Close()
	}()

	// sending SMS(s)
	for i, testCase := range testCases {
		color.Green("Test #%d:\n", i+1)

		if err = trans.Transceiver().Submit(newSubmitSM(testCase)); err != nil {
			color.Red("Error ", err)
		}
		time.Sleep(time.Second)
	}

	//for i := 0; i < len(testCases); i++ {
	//	if err = trans.Transceiver().Submit(newSubmitSM()); err != nil {
	//		color.Green(err)
	//	}
	//	time.Sleep(time.Second)
	//}
}

func handlePDU(trans **gosmpp.Session) func(pdu.PDU, bool) {
	concatenated := map[uint8][]string{}
	return func(p pdu.PDU, _ bool) {
		// Print out the received PDU type and details
		switch responsePdu := p.(type) {
		case *pdu.SubmitSMResp:
			// Track the response based on sequence number
			_, _ = requestTracker[responsePdu.SequenceNumber]
			testCase, testCaseExists := testCaseTracker[responsePdu.SequenceNumber]

			if testCaseExists {
				var expectedOutput = testCase.ExpectedOutput
				if int32(responsePdu.Header.CommandStatus) != int32(*expectedOutput.CommandStatus) {
					color.Red(
						"Test case failed for TestCase %d",
						testCase.TestCaseId,
					)
					color.Red(
						"CommandStatus mismatch with TestCase %d. Expected: %d, Got: %d\n",
						testCase.TestCaseId,
						*expectedOutput.CommandStatus,
						responsePdu.Header.CommandStatus,
					)
					os.Exit(1)
				}

			}
		case *pdu.GenericNack:
			color.Green("GenericNack Received")

		case *pdu.EnquireLinkResp:
			color.Green("EnquireLinkResp Received")

		case *pdu.DataSM:
			color.Green("DataSM:%+v\n", responsePdu)

		case *pdu.DeliverSM:
			color.Green("DeliverSM:%+v\n", responsePdu)
			color.Green(responsePdu.Message.GetMessage())
			message, err := responsePdu.Message.GetMessage()
			if err != nil {
				log.Fatal(err)
			}
			totalParts, sequence, reference, found := responsePdu.Message.UDH().GetConcatInfo()
			if found {
				if _, ok := concatenated[reference]; !ok {
					concatenated[reference] = make([]string, totalParts)
				}
				concatenated[reference][sequence-1] = message
			}
			if !found {
				color.Green(message)
			} else if parts, ok := concatenated[reference]; ok && isConcatenatedDone(parts, totalParts) {
				color.Green(strings.Join(parts, ""))
				delete(concatenated, reference)
			}
			// endregion

		case *pdu.UnbindResp:
			color.Green("UnbindResp:%+v\n", responsePdu)
			color.Green("UnbindResp received — closing session...")
			if *trans != nil {
				_ = (*trans).Close()
			}

		default:
			// Handling unhandled PDUs
			log.Printf("Unhandled PDU type: %T", responsePdu)
		}
	}
}

func newSubmitSM(testcase TestCase) *pdu.SubmitSM {
	requestPDU := testcase.InputPdu

	// build up submitSM
	srcAddr := pdu.NewAddress()

	srcAddr.SetTon(byte(*requestPDU.SourceAddrTON))
	srcAddr.SetNpi(byte(*requestPDU.SourceAddrNPI))
	_ = srcAddr.SetAddress(*requestPDU.SourceAddr)

	destAddr := pdu.NewAddress()
	if requestPDU.DestAddrTON != nil {
		destAddr.SetTon(byte(*requestPDU.DestAddrTON))
	}
	if requestPDU.DestAddrNPI != nil {
		destAddr.SetNpi(byte(*requestPDU.DestAddrNPI))
	}

	if requestPDU.DestinationAddr != nil {
		err := destAddr.SetAddress(*requestPDU.DestinationAddr)
		if err != nil {
			log.Fatal(err)
		}
	}

	submitSM := pdu.NewSubmitSM().(*pdu.SubmitSM)
	submitSM.SourceAddr = srcAddr
	submitSM.DestAddr = destAddr
	dataCode := byteToDataCoding(byte(*requestPDU.DataCoding))
	_ = submitSM.Message.SetMessageWithEncoding(*requestPDU.ShortMessage, dataCode)
	submitSM.ProtocolID = byte(*requestPDU.ProtocolID)
	submitSM.RegisteredDelivery = byte(*requestPDU.RegisteredDelivery)
	submitSM.ReplaceIfPresentFlag = byte(*requestPDU.ReplaceIfPresentFlag)
	submitSM.EsmClass = byte(*requestPDU.EsmClass)
	// Track the request by sequence_number
	requestTracker[submitSM.SequenceNumber] = submitSM
	testCaseTracker[submitSM.SequenceNumber] = &testcase

	return submitSM
}

func isConcatenatedDone(parts []string, total byte) bool {
	for _, part := range parts {
		if part != "" {
			total--
		}
	}
	return total == 0
}

// FromDataCodingExtended maps a Data Coding Scheme (DCS) byte value
// to its corresponding Encoding implementation, including GSM7BITPACKED.
func byteToDataCoding(code byte) data.Encoding {
	switch code {
	case data.UCS2Coding:
		return data.UCS2
	case data.HEBREWCoding:
		return data.HEBREW
	case data.CYRILLICCoding:
		return data.CYRILLIC
	case data.BINARY8BIT2Coding:
		return data.BINARY8BIT2
	case data.LATIN1Coding:
		return data.LATIN1
	case data.BINARY8BIT1Coding:
		return data.BINARY8BIT1
	case data.ASCIICoding:
		return data.ASCII
	case data.GSM7BITCoding:
		// You can choose to default to GSM7BIT or GSM7BITPACKED here.
		// Typically GSM7BIT is the standard, but if you need the packed variant, return GSM7BITPACKED instead.
		return data.GSM7BIT
	default:
		// Fallback: assume GSM7BITPACKED if unknown coding is given
		return data.GSM7BITPACKED
	}
}

func connectToHtppServerUsingTCP() {
	//	Connect securely to example.com on port 443 (HTTPS)

	conn, err := tls.Dial("tcp", "example.com:443", nil) // #A
	if err != nil {                                      // #B
		log.Fatal(err)
	}
	defer conn.Close()

	// Send an HTTPS GET request
	if _, err := fmt.Fprintf(conn, "GET / HTTP/1.1\r\nHost: example.com\r\n\r\n"); err != nil {
		log.Fatal(err)
	}

	// Read the first line of the response (status line)
	status, err := bufio.NewReader(conn).ReadString('\n') // #D
	if err != nil {                                       // #E
		log.Fatal(err)
	}

	// Print the HTTP status line
	color.Green(status) // #F

}

func validateResponseWithExpectedOutput(expectedOutput ExpectedOutput, responsePdu *pdu.SubmitSMResp) bool {
	if responsePdu == nil {
		return false
	}

	// Validate CommandID if expected
	//if expectedOutput.CommandID != nil {
	//	expected := *expectedOutput.CommandID
	//	if responsePdu.CommandID != nil {
	//		stringComparison(expected, responsePdu.CommandID.String())
	//		return false
	//	}
	//}

	//// Validate CommandStatus if expected
	//if expectedOutput.CommandStatus != nil {
	//	if responsePdu.CommandStatus != *expectedOutput.CommandStatus {
	//		return false
	//	}
	//}
	//
	//// Validate MessageID if expected
	//if expectedOutput.MessageID != nil {
	//	if responsePdu.MessageID != *expectedOutput.MessageID {
	//		return false
	//	}
	//}

	// DeliveryStatus, Segments, Error, Note are not part of SubmitSMResp directly,
	// so we only log or ignore them (depending on your requirements).
	// You could extend this check if those are set somewhere else in your system.

	return true
}

func stringComparison(expected, actual string) bool {
	expected = strings.ToLower(expected)
	actual = strings.ToLower(actual)

	found := strings.Contains(actual, expected)
	if found {
		color.Green("Substring found (case-insensitive)")
	}
	return found
}
