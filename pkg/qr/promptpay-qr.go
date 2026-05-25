package qr

import (
	"fmt"
	"strings"
)

// GeneratePromptPayBillPayment generates a raw EMVCo PromptPay QR content string.
// billerID: Tax ID + Suffix (e.g., "099400016485800")
// ref1: Reference 1 (up to 20 alphanumeric characters)
// ref2: Reference 2 (up to 20 alphanumeric characters)
// amount: Payment amount (float64)
func GeneratePromptPayBillPayment(billerID string, ref1 string, ref2 string, amount float64) (string, error) {
	// Clean ref values (uppercase, alphanumeric, remove spaces)
	ref1 = strings.TrimSpace(ref1)
	ref2 = strings.TrimSpace(ref2)

	// Format amount to 2 decimal places
	amountStr := fmt.Sprintf("%.2f", amount)

	// Build Merchant Account Information (Tag 30)
	// AID subtag 00: "A000000677010112"
	aidSub := "0016A000000677010112"
	// Biller ID subtag 01
	billerSub := fmt.Sprintf("01%02d%s", len(billerID), billerID)
	// Ref 1 subtag 02
	ref1Sub := fmt.Sprintf("02%02d%s", len(ref1), ref1)
	// Ref 2 subtag 03
	ref2Sub := ""
	if ref2 != "" {
		ref2Sub = fmt.Sprintf("03%02d%s", len(ref2), ref2)
	}

	tag30Value := aidSub + billerSub + ref1Sub + ref2Sub
	tag30 := fmt.Sprintf("30%02d%s", len(tag30Value), tag30Value)

	// Build basic EMVCo tags
	payload := "000201"  // Payload Format Indicator
	payload += "010212"  // Point of Initiation Method (12 = dynamic QR, 11 = static)
	payload += tag30
	payload += "5303764" // Currency (764 = THB)
	payload += fmt.Sprintf("54%02d%s", len(amountStr), amountStr)
	payload += "5802TH" // Country Code
	payload += "6304"   // CRC tag and length (pre-requisite for checksum calculation)

	// Compute CRC-16-CCITT
	checksum := CalculateCRC16(payload)

	return payload + checksum, nil
}

// CalculateCRC16 calculates the CRC-16-CCITT checksum (false, polynomial 0x1021, init 0xFFFF)
func CalculateCRC16(data string) string {
	var crc uint16 = 0xFFFF
	for i := 0; i < len(data); i++ {
		crc ^= uint16(data[i]) << 8
		for j := 0; j < 8; j++ {
			if (crc & 0x8000) != 0 {
				crc = (crc << 1) ^ 0x1021
			} else {
				crc <<= 1
			}
		}
	}
	return fmt.Sprintf("%04X", crc)
}
