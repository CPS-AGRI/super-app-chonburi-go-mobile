// scratch/verify_qr.go — Decode & Validate PromptPay Bill Payment QR
// Usage: go run scratch/verify_qr.go
//
// หรือใส่ QR string เองในตัวแปร qrString ด้านล่าง
// หรือให้ script ดึงจาก API declaration ล่าสุดอัตโนมัติ

package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strconv"
	"strings"
)

// ─────────────────────────────────────────────────────────────────────────────
// EMVCo QR Parser
// ─────────────────────────────────────────────────────────────────────────────

type TLVTag struct {
	Tag    string
	Length int
	Value  string
}

func parseTLV(qr string) []TLVTag {
	var tags []TLVTag
	i := 0
	for i < len(qr)-4 {
		tag := qr[i : i+2]
		lenStr := qr[i+2 : i+4]
		length, err := strconv.Atoi(lenStr)
		if err != nil || i+4+length > len(qr) {
			break
		}
		value := qr[i+4 : i+4+length]
		tags = append(tags, TLVTag{Tag: tag, Length: length, Value: value})
		i += 4 + length
	}
	return tags
}

// CalculateCRC16 — same algorithm as pkg/qr
func calculateCRC16(data string) string {
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

func tagName(tag string) string {
	names := map[string]string{
		"00": "Payload Format Indicator",
		"01": "Point of Initiation Method (11=static, 12=dynamic)",
		"30": "Merchant Account Information (PromptPay Bill Payment)",
		"53": "Transaction Currency (764=THB)",
		"54": "Transaction Amount",
		"58": "Country Code",
		"63": "CRC-16",
	}
	if name, ok := names[tag]; ok {
		return name
	}
	return "Unknown Tag"
}

func parseMerchantAccount(value string) {
	subtags := parseTLV(value)
	for _, sub := range subtags {
		switch sub.Tag {
		case "00":
			fmt.Printf("    AID              : %s\n", sub.Value)
		case "01":
			fmt.Printf("    Biller ID        : %s\n", sub.Value)
		case "02":
			fmt.Printf("    Ref1 (ทะเบียน+รหัส): %s\n", sub.Value)
		case "03":
			fmt.Printf("    Ref2 (ปี+เดือน+ver): %s\n", sub.Value)
		default:
			fmt.Printf("    [%s] %s\n", sub.Tag, sub.Value)
		}
	}
}

func decodeQR(qrString string) bool {
	if qrString == "" {
		fmt.Println("❌ QR string ว่างเปล่า")
		return false
	}

	fmt.Println()
	fmt.Println("══════════════════════════════════════════════════════════════")
	fmt.Println("  🔍 DECODE PromptPay Bill Payment QR (EMVCo)")
	fmt.Println("══════════════════════════════════════════════════════════════")
	fmt.Printf("  QR String (%d chars): %s\n", len(qrString), qrString)
	fmt.Println("──────────────────────────────────────────────────────────────")

	// Validate CRC
	if len(qrString) < 4 {
		fmt.Println("❌ QR string สั้นเกินไป")
		return false
	}
	crcData := qrString[:len(qrString)-4]
	crcInQR := qrString[len(qrString)-4:]
	crcCalculated := calculateCRC16(crcData)
	crcOK := strings.EqualFold(crcInQR, crcCalculated)

	fmt.Printf("  CRC-16 ใน QR     : %s\n", crcInQR)
	fmt.Printf("  CRC-16 คำนวณใหม่ : %s\n", crcCalculated)
	if crcOK {
		fmt.Println("  ✅ CRC VALID — QR นี้ถูกต้องตาม EMVCo spec!")
	} else {
		fmt.Println("  ❌ CRC MISMATCH — QR อาจถูกแก้ไขหรือมีข้อผิดพลาด")
	}
	fmt.Println("──────────────────────────────────────────────────────────────")

	// Parse all TLV tags
	tags := parseTLV(qrString)
	fmt.Println("  📋 รายละเอียดในแต่ละ Tag:")
	for _, tag := range tags {
		fmt.Printf("\n  [Tag %s] %s\n", tag.Tag, tagName(tag.Tag))
		fmt.Printf("    Value: %s\n", tag.Value)
		if tag.Tag == "30" {
			parseMerchantAccount(tag.Value)
		}
	}

	fmt.Println()
	fmt.Println("══════════════════════════════════════════════════════════════")
	return crcOK
}

// ─────────────────────────────────────────────────────────────────────────────
// Fetch QR from API (latest declaration)
// ─────────────────────────────────────────────────────────────────────────────

func fetchQRFromAPI(baseURL, declarationID string) string {
	url := fmt.Sprintf("%s/api/v1/tax-new/declare/%s", baseURL, declarationID)
	fmt.Printf("📡 เรียก API: GET %s\n", url)

	resp, err := http.Get(url)
	if err != nil {
		fmt.Printf("❌ ไม่สามารถเชื่อมต่อ API: %v\n", err)
		return ""
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)

	var result struct {
		Success bool `json:"success"`
		Data    struct {
			QRCodeContent *string `json:"qr_code_content"`
			DeclarationID string  `json:"declaration_id"`
			BusinessName  string  `json:"business_name"`
			TaxMonth      int     `json:"tax_month"`
			TaxYear       int     `json:"tax_year"`
			CalculatedTax float64 `json:"calculated_tax"`
			PaymentStatus string  `json:"payment_status"`
			Ref1          string  `json:"ref1"`
			Ref2          string  `json:"ref2"`
		} `json:"data"`
	}

	if err := json.Unmarshal(body, &result); err != nil || !result.Success {
		fmt.Printf("❌ API Response ผิดพลาด: %s\n", string(body))
		return ""
	}

	d := result.Data
	fmt.Println()
	fmt.Println("  📄 ข้อมูล Declaration จาก API:")
	fmt.Printf("  Declaration ID : %s\n", d.DeclarationID)
	fmt.Printf("  ชื่อสถานประกอบการ: %s\n", d.BusinessName)
	fmt.Printf("  รอบภาษี        : %02d/%04d\n", d.TaxMonth, d.TaxYear)
	fmt.Printf("  ยอดภาษี        : %.2f บาท\n", d.CalculatedTax)
	fmt.Printf("  สถานะ          : %s\n", d.PaymentStatus)
	fmt.Printf("  Ref1 / Ref2    : %s / %s\n", d.Ref1, d.Ref2)

	if d.QRCodeContent == nil || *d.QRCodeContent == "" {
		fmt.Println("  ⚠️  QR Code Content ว่างเปล่าใน DB")
		return ""
	}
	return *d.QRCodeContent
}

// ─────────────────────────────────────────────────────────────────────────────
// Main
// ─────────────────────────────────────────────────────────────────────────────

func main() {
	baseURL := "http://localhost:8082"

	// ── Option A: วาง Declaration ID ที่ต้องการทดสอบตรงนี้
	declarationID := ""

	// ── Option B: รับจาก command line argument
	// go run scratch/verify_qr.go <declaration-id>
	if len(os.Args) > 1 {
		declarationID = os.Args[1]
	}

	// ── Option C: วาง QR string โดยตรง (ไม่ผ่าน API)
	manualQR := ""
	// manualQR = "000201010212303..."  // ← ใส่ QR string ตรงนี้ได้เลย

	var qrString string
	if manualQR != "" {
		fmt.Println("🔧 Mode: Manual QR String")
		qrString = manualQR
	} else if declarationID != "" {
		fmt.Printf("🔧 Mode: Fetch from API — Declaration ID: %s\n", declarationID)
		qrString = fetchQRFromAPI(baseURL, declarationID)
	} else {
		fmt.Println("⚠️  ไม่ได้ระบุ Declaration ID")
		fmt.Println()
		fmt.Println("วิธีใช้:")
		fmt.Println("  go run scratch/verify_qr.go <declaration-id>")
		fmt.Println()
		fmt.Println("ตัวอย่าง:")
		fmt.Println("  go run scratch/verify_qr.go f47ac10b-58cc-4372-a567-0e02b2c3d479")
		fmt.Println()
		fmt.Println("หรือแก้ไข manualQR ในไฟล์นี้แล้วรัน:")
		fmt.Println("  go run scratch/verify_qr.go")
		os.Exit(1)
	}

	ok := decodeQR(qrString)
	if !ok {
		os.Exit(1)
	}
}
