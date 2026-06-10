package usecase

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"super-app-chonburi-go-mobile/internal/domain"
	"super-app-chonburi-go-mobile/pkg/mail"
	"super-app-chonburi-go-mobile/pkg/qr"

	"github.com/google/uuid"
)

type taxNewMobileUseCase struct {
	repo       domain.TaxNewMobileRepository
	mailSender mail.EmailSender
	billerID   string
}

func NewTaxNewMobileUseCase(repo domain.TaxNewMobileRepository, mailSender mail.EmailSender, billerID string) domain.TaxNewMobileUseCase {
	if billerID == "" {
		billerID = "099400016485800"
	}
	return &taxNewMobileUseCase{
		repo:       repo,
		mailSender: mailSender,
		billerID:   billerID,
	}
}

func (u *taxNewMobileUseCase) GetBusiness(regNumber string) (*domain.TaxBusinessDTO, error) {
	business, err := u.repo.GetBusinessByRegNumber(regNumber)
	if err != nil {
		return nil, err
	}
	if business == nil {
		return nil, errors.New("business not found")
	}

	rate, err := u.repo.GetActiveTaxRate(business.TaxType)
	if err != nil {
		return nil, err
	}
	rateValue := 0.0
	rateUnit := "percentage"
	if rate != nil {
		rateValue = rate.RateValue
		rateUnit = rate.RateUnit
	}

	now := time.Now()
	currentMonth := int(now.Month())
	currentYear := now.Year()

	hasPaid, err := u.repo.HasPaidDeclaration(business.BusinessRegNumber, currentMonth, currentYear)
	if err != nil {
		return nil, err
	}

	return &domain.TaxBusinessDTO{
		BusinessRegNumber: business.BusinessRegNumber,
		NameTH:            business.NameTH,
		TaxType:           business.TaxType,
		TaxRate:           rateValue,
		RateUnit:          rateUnit,
		HasPaidThisMonth:  hasPaid,
	}, nil
}

func (u *taxNewMobileUseCase) DeclareTax(req domain.DeclareTaxRequest) (*domain.DeclareTaxResponse, error) {
	business, err := u.repo.GetBusinessByRegNumber(req.BusinessRegNumber)
	if err != nil {
		return nil, err
	}
	if business == nil {
		return nil, errors.New("business not found")
	}

	rate, err := u.repo.GetActiveTaxRate(business.TaxType)
	if err != nil {
		return nil, err
	}
	if rate == nil {
		return nil, errors.New("active tax rate not found for tax type " + business.TaxType)
	}

	calculatedTax := req.MonthlyRevenue * rate.RateValue
	if rate.RateUnit == "percentage" {
		calculatedTax = req.MonthlyRevenue * (rate.RateValue / 100.0)
	}

	version, err := u.repo.GetLatestDeclarationVersion(req.BusinessRegNumber, business.TaxType, req.TaxMonth, req.TaxYear)
	if err != nil {
		return nil, err
	}
	newVersion := version + 1

	var typeCode string
	switch business.TaxType {
	case "hotel_fee":
		typeCode = "01"
	case "oil_gas_tax":
		typeCode = "02"
	case "tobacco_tax":
		typeCode = "03"
	default:
		typeCode = "00"
	}
	ref1 := fmt.Sprintf("%s%s", business.BusinessRegNumber, typeCode)
	ref2 := fmt.Sprintf("%04d%02d%02d", req.TaxYear, req.TaxMonth, newVersion)

	qrContent, err := qr.GeneratePromptPayBillPayment(u.billerID, ref1, ref2, calculatedTax)
	if err != nil {
		return nil, fmt.Errorf("failed to generate promptpay QR: %w", err)
	}

	declaration := &domain.TaxDeclaration{
		ID:                 uuid.New(),
		BusinessID:         business.ID,
		BusinessRegNumber:  business.BusinessRegNumber,
		TaxType:            business.TaxType,
		TaxMonth:           req.TaxMonth,
		TaxYear:            req.TaxYear,
		DeclarationVersion: newVersion,
		MonthlyRevenue:     req.MonthlyRevenue,
		VolumeUnits:        req.VolumeUnits,
		CalculatedTax:      calculatedTax,
		FormFileURL:        req.FormFileURL,
		PayerEmail:         req.PayerEmail,
		PayerPhone:         &req.PayerPhone,
		Ref1:               ref1,
		Ref2:               ref2,
		QRCodeContent:      &qrContent,
		PaymentStatus:      "pending",
		CreatedAt:          time.Now(),
		UpdatedAt:          time.Now(),
	}

	if err := u.repo.CreateDeclaration(declaration); err != nil {
		return nil, err
	}

	SendNotificationToDepartment(
		"",
		"officer",
		"มีรายการยื่นแบบภาษีใหม่",
		fmt.Sprintf("สถานประกอบการ %s ได้ยื่นแบบภาษี %s รอบประจำเดือน %s %d ยอดภาษีคำนวณ %s บาท รอตอบรับ",
			business.NameTH, getTaxTypeNameTH(declaration.TaxType), getThaiMonthName(declaration.TaxMonth), declaration.TaxYear+543, formatWithCommas(declaration.CalculatedTax)),
		declaration.ID.String(),
		"pending",
	)

	return &domain.DeclareTaxResponse{
		DeclarationID: declaration.ID,
		CalculatedTax: declaration.CalculatedTax,
		Ref1:          declaration.Ref1,
		Ref2:          declaration.Ref2,
		QRCodeContent: *declaration.QRCodeContent,
		PaymentStatus: declaration.PaymentStatus,
	}, nil
}

func (u *taxNewMobileUseCase) GetDeclaration(id uuid.UUID) (*domain.TaxDeclaration, error) {
	return u.repo.GetDeclarationByID(id)
}

func getThaiMonthName(m int) string {
	months := []string{"", "มกราคม", "กุมภาพันธ์", "มีนาคม", "เมษายน", "พฤษภาคม", "มิถุนายน", "กรกฎาคม", "สิงหาคม", "กันยายน", "ตุลาคม", "พฤศจิกายน", "ธันวาคม"}
	if m >= 1 && m <= 12 {
		return months[m]
	}
	return ""
}

func getTaxTypeNameTH(t string) string {
	switch t {
	case "hotel_fee":
		return "ค่าธรรมเนียมบำรุง อบจ. จากผู้เข้าพักโรงแรม"
	case "oil_gas_tax":
		return "ภาษีบำรุง อบจ.จากการค้าน้ำมัน/ก๊าซ"
	case "tobacco_tax":
		return "ภาษีบำรุง อบจ.จากการค้ายาสูบ"
	default:
		return "ภาษี/ค่าธรรมเนียม"
	}
}

func formatWithCommas(val float64) string {
	parts := strings.Split(fmt.Sprintf("%.2f", val), ".")
	intPart := parts[0]
	decPart := parts[1]

	var result []string
	length := len(intPart)
	for i, char := range intPart {
		if i > 0 && (length-i)%3 == 0 && intPart[i-1] != '-' {
			result = append(result, ",")
		}
		result = append(result, string(char))
	}
	return strings.Join(result, "") + "." + decPart
}

func (u *taxNewMobileUseCase) sendPaymentSuccessEmail(decl *domain.TaxDeclaration) {
	if decl.Business == nil {
		return
	}
	subject := fmt.Sprintf("ยืนยันการชำระเงินภาษี/ค่าธรรมเนียม อบจ. ชลบุรี - %s", decl.Business.NameTH)

	thaiMonth := getThaiMonthName(decl.TaxMonth)
	thaiYear := decl.TaxYear + 543

	paidAt := ""
	if decl.PaidAt != nil {
		paidAt = decl.PaidAt.Format("02/01/2006 15:04:05")
	}

	body := fmt.Sprintf(`
	<html>
	<body style="font-family: 'Helvetica Neue', Helvetica, Arial, sans-serif; color: #333; line-height: 1.6;">
		<div style="max-width: 600px; margin: 0 auto; padding: 20px; border: 1px solid #e0e0e0; border-radius: 8px;">
			<div style="text-align: center; margin-bottom: 20px;">
				<h2 style="color: #1a73e8; margin-top: 10px;">ใบเสร็จรับเงิน / ยืนยันการชำระเงิน</h2>
				<p style="color: #666; font-size: 14px;">องค์การบริหารส่วนจังหวัดชลบุรี</p>
			</div>
			<hr style="border: 0; border-top: 1px solid #eee; margin: 20px 0;">
			<p>เรียน ผู้เสียภาษี,</p>
			<p>ระบบงานภาษีและค่าธรรมเนียม อบจ. ชลบุรี ได้รับเงินโอนค่าภาษี/ค่าธรรมเนียมของท่านเรียบร้อยแล้ว รายละเอียดดังนี้:</p>
			<table style="width: 100%%; border-collapse: collapse; margin: 20px 0;">
				<tr><td style="padding: 8px 0; font-weight: bold; width: 180px;">รหัสสถานประกอบการ:</td><td style="padding: 8px 0;">%s</td></tr>
				<tr><td style="padding: 8px 0; font-weight: bold;">ชื่อสถานประกอบการ:</td><td style="padding: 8px 0;">%s</td></tr>
				<tr><td style="padding: 8px 0; font-weight: bold;">ประเภทภาษี/ค่าธรรมเนียม:</td><td style="padding: 8px 0;">%s</td></tr>
				<tr><td style="padding: 8px 0; font-weight: bold;">รอบภาษีประจำเดือน:</td><td style="padding: 8px 0;">%s %d (เวอร์ชันการยื่นที่ %d)</td></tr>
				<tr><td style="padding: 8px 0; font-weight: bold;">ยอดเงินที่ชำระ:</td><td style="padding: 8px 0; font-size: 18px; color: #2e7d32; font-weight: bold;">%s บาท</td></tr>
				<tr><td style="padding: 8px 0; font-weight: bold;">วันที่ชำระเงิน:</td><td style="padding: 8px 0;">%s</td></tr>
				<tr><td style="padding: 8px 0; font-weight: bold;">เลขอ้างอิง Ref 1 / Ref 2:</td><td style="padding: 8px 0; font-family: monospace;">%s / %s</td></tr>
			</table>
			<div style="background-color: #f1f8e9; border-left: 4px solid #8bc34a; padding: 15px; border-radius: 4px; margin-top: 20px;">
				<p style="margin: 0; font-size: 14px; color: #33691e;"><strong>หมายเหตุ:</strong> เอกสารใบเสร็จอย่างเป็นทางการของ อบจ. ชลบุรี จะถูกส่งมอบให้ท่านทางไปรษณีย์หรือช่องทางที่ท่านลงทะเบียนไว้ต่อไป</p>
			</div>
			<hr style="border: 0; border-top: 1px solid #eee; margin: 20px 0;">
			<p style="font-size: 12px; color: #999; text-align: center;">นี่เป็นอีเมลอัตโนมัติ กรุณาอย่าตอบกลับอีเมลฉบับนี้</p>
		</div>
	</body>
	</html>`,
		decl.BusinessRegNumber,
		decl.Business.NameTH,
		getTaxTypeNameTH(decl.TaxType),
		thaiMonth,
		thaiYear,
		decl.DeclarationVersion,
		formatWithCommas(decl.CalculatedTax),
		paidAt,
		decl.Ref1,
		decl.Ref2,
	)

	_ = u.mailSender.SendHTML([]string{decl.PayerEmail}, subject, body)
}
