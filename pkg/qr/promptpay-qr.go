package qr

import (
	"fmt"
	"strings"
)

func GeneratePromptPayBillPayment(billerID string, ref1 string, ref2 string, amount float64) (string, error) {

	ref1 = strings.TrimSpace(ref1)
	ref2 = strings.TrimSpace(ref2)

	amountStr := fmt.Sprintf("%.2f", amount)

	aidSub := "0016A000000677010112"

	billerSub := fmt.Sprintf("01%02d%s", len(billerID), billerID)

	ref1Sub := fmt.Sprintf("02%02d%s", len(ref1), ref1)

	ref2Sub := ""
	if ref2 != "" {
		ref2Sub = fmt.Sprintf("03%02d%s", len(ref2), ref2)
	}

	tag30Value := aidSub + billerSub + ref1Sub + ref2Sub
	tag30 := fmt.Sprintf("30%02d%s", len(tag30Value), tag30Value)

	payload := "000201"
	payload += "010212"
	payload += tag30
	payload += "5303764"
	payload += fmt.Sprintf("54%02d%s", len(amountStr), amountStr)
	payload += "5802TH"
	payload += "6304"

	checksum := CalculateCRC16(payload)

	return payload + checksum, nil
}

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
