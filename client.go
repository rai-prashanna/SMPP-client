package main

import (
	"fmt"
	"log"
	"strings"
	"sync"

	"github.com/linxGnu/gosmpp"
	"github.com/linxGnu/gosmpp/pdu"
)

// Client encapsulates an SMPP session and PDU handling.
type Client struct {
	cfg          Config
	session      *gosmpp.Session
	concatMu     sync.Mutex
	concatenated map[uint8][]string
}

// NewClient creates a new Client with given configuration.
func NewClient(cfg Config) *Client {
	return &Client{
		cfg:          cfg,
		concatenated: make(map[uint8][]string),
	}
}

// Connect starts the SMPP session.
func (c *Client) Connect() error {
	auth := gosmpp.Auth{
		SMSC:       fmt.Sprintf("%s:%s", c.cfg.Host, c.cfg.Port),
		SystemID:   c.cfg.SystemID,
		Password:   c.cfg.Password,
		SystemType: c.cfg.SystemType,
	}

	connector := gosmpp.TRXConnector(TLSDialer, auth)

	settings := gosmpp.Settings{
		EnquireLink: c.cfg.EnquireLink,
		ReadTimeout: c.cfg.ReadTimeout,
		OnSubmitError: func(_ pdu.PDU, err error) {
			log.Printf("SubmitPDU error: %v", err)
		},
		OnReceivingError: func(err error) {
			log.Printf("Receiving PDU/Network error: %v", err)
		},
		OnRebindingError: func(err error) {
			log.Printf("Rebinding error: %v", err)
		},
		OnPDU:    c.onPDU,
		OnClosed: func(state gosmpp.State) { log.Printf("SMPP connection closed: %v", state) },
	}

	session, err := gosmpp.NewSession(connector, settings, c.cfg.ReadTimeout)
	if err != nil {
		return err
	}
	c.session = session
	return nil
}

// Close closes the session.
func (c *Client) Close() error {
	if c.session == nil {
		return nil
	}
	return c.session.Close()
}

// SendSMS submits a SubmitSM PDU via the session transceiver.
func (c *Client) SendSMS(sm *pdu.SubmitSM) error {
	if c.session == nil {
		return fmt.Errorf("session not connected")
	}
	return c.session.Transceiver().Submit(sm)
}

// onPDU handles incoming PDUs.
func (c *Client) onPDU(p pdu.PDU, _ bool) {
	switch pd := p.(type) {
	case *pdu.SubmitSMResp:
		log.Printf("SubmitSMResp: %+v", pd)
	case *pdu.GenericNack:
		log.Println("GenericNack Received")
	case *pdu.EnquireLinkResp:
		log.Println("EnquireLinkResp Received")
	case *pdu.DataSM:
		log.Printf("DataSM: %+v", pd)
	case *pdu.DeliverSM:
		log.Printf("DeliverSM: %+v", pd)
		message, err := pd.Message.GetMessage()
		if err != nil {
			log.Printf("failed to get message: %v", err)
			return
		}
		totalParts, sequence, reference, found := pd.Message.UDH().GetConcatInfo()
		if found {
			c.concatMu.Lock()
			if _, ok := c.concatenated[reference]; !ok {
				c.concatenated[reference] = make([]string, totalParts)
			}
			c.concatenated[reference][sequence-1] = message
			done := isConcatenatedDone(c.concatenated[reference], totalParts)
			c.concatMu.Unlock()
			if done {
				c.concatMu.Lock()
				parts := c.concatenated[reference]
				delete(c.concatenated, reference)
				c.concatMu.Unlock()
				log.Println("Reassembled (concatenated) message:", strings.Join(parts, ""))
			} else {
				log.Printf("Stored part %d/%d for reference %d", sequence, totalParts, reference)
			}
		} else {
			log.Println("Message:", message)
		}
	default:
		log.Printf("Unhandled PDU type: %T", pd)
		log.Printf("Closing session: %T", pd)
		err := c.Close()
		if err != nil {
			log.Printf("unable to close session: %T", pd)

			return
		}
		log.Printf("CLosed session: %T", pd)

	}
}
