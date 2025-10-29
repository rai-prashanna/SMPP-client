package main

import (
	"github.com/linxGnu/gosmpp/data"
	"github.com/linxGnu/gosmpp/pdu"
)

// NewSubmitSM constructs a SubmitSM PDU with basic fields.
// It uses UCS2 encoding for the message like the original sample.
func NewSubmitSM(src, dest, message string) *pdu.SubmitSM {
	srcAddr := pdu.NewAddress()
	srcAddr.SetTon(5)
	srcAddr.SetNpi(0)
	_ = srcAddr.SetAddress(src)

	destAddr := pdu.NewAddress()
	destAddr.SetTon(1)
	destAddr.SetNpi(1)
	_ = destAddr.SetAddress(dest)

	submit := pdu.NewSubmitSM().(*pdu.SubmitSM)
	submit.SourceAddr = srcAddr
	submit.DestAddr = destAddr
	_ = submit.Message.SetMessageWithEncoding(message, data.UCS2)
	submit.ProtocolID = 0
	submit.RegisteredDelivery = 1
	submit.ReplaceIfPresentFlag = 0
	submit.EsmClass = 0

	return submit
}
