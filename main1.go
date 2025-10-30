package main

import (
	"crypto/tls"
	_ "encoding/json"
	"fmt"
	"log"
	"net"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/joho/godotenv"
	"github.com/linxGnu/gosmpp"
	"github.com/linxGnu/gosmpp/data"
	"github.com/linxGnu/gosmpp/pdu"
)

var requestTracker = make(map[int32]*pdu.SubmitSM) // Track request based on sequence_number

func main() {
	var wg sync.WaitGroup

	wg.Add(1)
	go sendingAndReceiveSMS(&wg)

	wg.Wait()
}

func sendingAndReceiveSMS(wg *sync.WaitGroup) {
	defer wg.Done()
	_ = godotenv.Load() // ignore error; .env is optional
	server := os.Getenv("SMPP_HOST") + ":" + os.Getenv("SMPP_PORT")
	systemId := os.Getenv("SYSTEM_ID")
	password := os.Getenv("PASSWORD")

	testCases, parserError := parseFile("test-case.jsonl")

	if parserError != nil {
		fmt.Println("Rebinding but error:", parserError)
		//fmt.Fprintf(os.Stderr, "error parsing file: %v\n", parserError)
		os.Exit(1)
	}

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
				fmt.Println("Receiving PDU/Network error:", err)
			},

			OnRebindingError: func(err error) {
				fmt.Println("Rebinding but error:", err)
			},

			OnPDU: handlePDU(&trans),

			OnClosed: func(state gosmpp.State) {
				fmt.Println(state)
			},
		}, 5*time.Second)

	if err != nil {
		log.Fatal(err)
	}

	defer func() {
		_ = trans.Close()
	}()

	// sending SMS(s)
	for i, _ := range testCases {
		fmt.Printf("Test #%d:\n", i+1)
		//fmt.Printf("Test #%s:\n", testCase)
		if err = trans.Transceiver().Submit(newSubmitSM()); err != nil {
			fmt.Println(err)
		}
		time.Sleep(time.Second)
	}

	//for i := 0; i < len(testCases); i++ {
	//	if err = trans.Transceiver().Submit(newSubmitSM()); err != nil {
	//		fmt.Println(err)
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
			requestPDU, exists := requestTracker[responsePdu.SequenceNumber]
			if exists {
				log.Printf("Request: %+v\n", requestPDU) // Print the corresponding request details

				log.Printf("Received SubmitSMResp for SequenceNumber %+v\n", responsePdu)
			} else {
				log.Printf("No matching SubmitSM request for SequenceNumber %+v\n", responsePdu)
			}
		case *pdu.GenericNack:
			fmt.Println("GenericNack Received")

		case *pdu.EnquireLinkResp:
			fmt.Println("EnquireLinkResp Received")

		case *pdu.DataSM:
			fmt.Printf("DataSM:%+v\n", responsePdu)

		case *pdu.DeliverSM:
			fmt.Printf("DeliverSM:%+v\n", responsePdu)
			log.Println(responsePdu.Message.GetMessage())
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
				log.Println(message)
			} else if parts, ok := concatenated[reference]; ok && isConcatenatedDone(parts, totalParts) {
				log.Println(strings.Join(parts, ""))
				delete(concatenated, reference)
			}
			// endregion

		case *pdu.UnbindResp:
			fmt.Printf("UnbindResp:%+v\n", responsePdu)
			fmt.Println("UnbindResp received — closing session...")
			if *trans != nil {
				_ = (*trans).Close()
			}

		default:
			// Handling unhandled PDUs
			log.Printf("Unhandled PDU type: %T", responsePdu)
		}
	}
}

func newSubmitSM() *pdu.SubmitSM {
	// build up submitSM
	srcAddr := pdu.NewAddress()
	srcAddr.SetTon(1)
	srcAddr.SetNpi(0)
	_ = srcAddr.SetAddress("00")

	destAddr := pdu.NewAddress()
	destAddr.SetTon(1)
	destAddr.SetNpi(1)
	_ = destAddr.SetAddress("99" + "522241")

	submitSM := pdu.NewSubmitSM().(*pdu.SubmitSM)
	submitSM.SourceAddr = srcAddr
	submitSM.DestAddr = destAddr
	_ = submitSM.Message.SetMessageWithEncoding("HELLO WORLD!", data.UCS2)
	submitSM.ProtocolID = 0
	submitSM.RegisteredDelivery = 1
	submitSM.ReplaceIfPresentFlag = 0
	submitSM.EsmClass = 0
	// Track the request by sequence_number
	requestTracker[submitSM.SequenceNumber] = submitSM
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
