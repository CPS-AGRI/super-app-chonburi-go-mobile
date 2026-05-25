package domain

import (
	"time"

	"github.com/google/uuid"
)

// ─────────────────────────────────────────────────────────────────────────────
// GORM Models (shared DB — schema owned by super-app-chonburi-go)
// ─────────────────────────────────────────────────────────────────────────────

// TaxRate represents tax_rates table
type TaxRate struct {
	ID        uuid.UUID `gorm:"type:uuid;primaryKey;column:id;default:uuid_generate_v4()" json:"id"`
	TaxType   string    `gorm:"type:varchar(50);not null;unique;column:tax_type" json:"tax_type"` // 'hotel_fee', 'oil_gas_tax', 'tobacco_tax'
	NameTH    string    `gorm:"type:varchar(100);not null;column:name_th" json:"name_th"`
	RateValue float64   `gorm:"type:numeric(10,4);not null;column:rate_value" json:"rate_value"`
	RateUnit  string    `gorm:"type:varchar(20);not null;column:rate_unit" json:"rate_unit"` // 'percentage', 'per_litre', 'per_pack'
	IsActive  bool      `gorm:"type:boolean;not null;default:true;column:is_active" json:"is_active"`
	CreatedAt time.Time `gorm:"type:timestamptz;not null;default:now();column:created_at" json:"created_at"`
	UpdatedAt time.Time `gorm:"type:timestamptz;not null;default:now();column:updated_at" json:"updated_at"`
}

func (TaxRate) TableName() string { return "tax_rates" }

// TaxBusiness represents tax_businesses table
type TaxBusiness struct {
	ID                  uuid.UUID `gorm:"type:uuid;primaryKey;column:id;default:uuid_generate_v4()" json:"id"`
	BusinessRegNumber   string    `gorm:"type:varchar(20);not null;unique;index;column:business_reg_number" json:"business_reg_number"`
	NameTH              string    `gorm:"type:varchar(255);not null;column:name_th" json:"name_th"`
	NameEN              *string   `gorm:"type:varchar(255);column:name_en" json:"name_en,omitempty"`
	TaxType             string    `gorm:"type:varchar(50);not null;column:tax_type" json:"tax_type"`
	OwnerName           *string   `gorm:"type:varchar(255);column:owner_name" json:"owner_name,omitempty"`
	OwnerIdentityNumber *string   `gorm:"type:varchar(13);column:owner_identity_number" json:"owner_identity_number,omitempty"`
	ContactPhone        *string   `gorm:"type:varchar(50);column:contact_phone" json:"contact_phone,omitempty"`
	ContactEmail        *string   `gorm:"type:varchar(255);column:contact_email" json:"contact_email,omitempty"`
	AddressDetail       *string   `gorm:"type:text;column:address_detail" json:"address_detail,omitempty"`
	CreatedAt           time.Time `gorm:"type:timestamptz;not null;default:now();column:created_at" json:"created_at"`
	UpdatedAt           time.Time `gorm:"type:timestamptz;not null;default:now();column:updated_at" json:"updated_at"`
}

func (TaxBusiness) TableName() string { return "tax_businesses" }

// TaxDeclaration represents tax_declarations table
type TaxDeclaration struct {
	ID                        uuid.UUID  `gorm:"type:uuid;primaryKey;column:id;default:uuid_generate_v4()" json:"id"`
	BusinessID                uuid.UUID  `gorm:"type:uuid;not null;column:business_id" json:"business_id"`
	BusinessRegNumber         string     `gorm:"type:varchar(20);not null;uniqueIndex:idx_tax_period;column:business_reg_number" json:"business_reg_number"`
	TaxType                   string     `gorm:"type:varchar(50);not null;uniqueIndex:idx_tax_period;column:tax_type" json:"tax_type"`
	TaxMonth                  int        `gorm:"type:integer;not null;uniqueIndex:idx_tax_period;column:tax_month" json:"tax_month"`
	TaxYear                   int        `gorm:"type:integer;not null;uniqueIndex:idx_tax_period;column:tax_year" json:"tax_year"`
	DeclarationVersion        int        `gorm:"type:integer;not null;default:1;uniqueIndex:idx_tax_period;column:declaration_version" json:"declaration_version"`
	MonthlyRevenue            float64    `gorm:"type:numeric(15,2);not null;column:monthly_revenue" json:"monthly_revenue"`
	VolumeUnits               float64    `gorm:"type:numeric(15,4);default:0.0;column:volume_units" json:"volume_units"`
	CalculatedTax             float64    `gorm:"type:numeric(12,2);not null;column:calculated_tax" json:"calculated_tax"`
	FormFileURL               string     `gorm:"type:varchar(512);not null;column:form_file_url" json:"form_file_url"`
	PayerEmail                string     `gorm:"type:varchar(255);not null;column:payer_email" json:"payer_email"`
	PayerPhone                *string    `gorm:"type:varchar(50);column:payer_phone" json:"payer_phone,omitempty"`
	Ref1                      string     `gorm:"type:varchar(20);not null;index;column:ref1" json:"ref1"`
	Ref2                      string     `gorm:"type:varchar(20);not null;index;column:ref2" json:"ref2"`
	QRCodeContent             *string    `gorm:"type:text;column:qr_code_content" json:"qr_code_content,omitempty"`
	PaymentStatus             string     `gorm:"type:varchar(30);not null;default:'pending';index;column:payment_status" json:"payment_status"` // 'pending', 'paid', 'verified', 'audit_failed'
	PaidAmount                *float64   `gorm:"type:numeric(12,2);column:paid_amount" json:"paid_amount,omitempty"`
	PaidAt                    *time.Time `gorm:"type:timestamptz;column:paid_at" json:"paid_at,omitempty"`
	KTBReconciliationRecordID *uuid.UUID `gorm:"type:uuid;column:ktb_reconciliation_record_id" json:"ktb_reconciliation_record_id,omitempty"`
	CreatedAt                 time.Time  `gorm:"type:timestamptz;not null;default:now();column:created_at" json:"created_at"`
	UpdatedAt                 time.Time  `gorm:"type:timestamptz;not null;default:now();column:updated_at" json:"updated_at"`

	// Relations
	Business *TaxBusiness `gorm:"foreignKey:BusinessID" json:"business,omitempty"`
}

func (TaxDeclaration) TableName() string { return "tax_declarations" }

// BankReconciliationBatch represents bank_reconciliation_batches table
type BankReconciliationBatch struct {
	ID           uuid.UUID `gorm:"type:uuid;primaryKey;column:id;default:uuid_generate_v4()" json:"id"`
	Filename     string    `gorm:"type:varchar(255);not null;column:filename" json:"filename"`
	UploadedBy   uuid.UUID `gorm:"type:uuid;not null;column:uploaded_by" json:"uploaded_by"`
	RecordCount  int       `gorm:"type:integer;not null;default:0;column:record_count" json:"record_count"`
	MatchedCount int       `gorm:"type:integer;not null;default:0;column:matched_count" json:"matched_count"`
	TotalAmount  float64   `gorm:"type:numeric(15,2);not null;default:0.00;column:total_amount" json:"total_amount"`
	CreatedAt    time.Time `gorm:"type:timestamptz;not null;default:now();column:created_at" json:"created_at"`
}

func (BankReconciliationBatch) TableName() string { return "bank_reconciliation_batches" }

// BankReconciliationRecord represents bank_reconciliation_records table
type BankReconciliationRecord struct {
	ID          uuid.UUID `gorm:"type:uuid;primaryKey;column:id;default:uuid_generate_v4()" json:"id"`
	BatchID     uuid.UUID `gorm:"type:uuid;not null;column:batch_id" json:"batch_id"`
	Ref1        string    `gorm:"type:varchar(50);not null;index:idx_bank_recon_refs;column:ref1" json:"ref1"`
	Ref2        string    `gorm:"type:varchar(50);not null;index:idx_bank_recon_refs;column:ref2" json:"ref2"`
	Amount      float64   `gorm:"type:numeric(12,2);not null;column:amount" json:"amount"`
	PaymentDate time.Time `gorm:"type:timestamptz;not null;column:payment_date" json:"payment_date"`
	RawLine     string    `gorm:"type:text;not null;column:raw_line" json:"raw_line"`
	IsMatched   bool      `gorm:"type:boolean;not null;default:false;index:idx_bank_recon_refs;column:is_matched" json:"is_matched"`
	CreatedAt   time.Time `gorm:"type:timestamptz;not null;default:now();column:created_at" json:"created_at"`
}

func (BankReconciliationRecord) TableName() string { return "bank_reconciliation_records" }

// ElaasDailySummary represents elaas_daily_summaries table
type ElaasDailySummary struct {
	ID               uuid.UUID `gorm:"type:uuid;primaryKey;column:id;default:uuid_generate_v4()" json:"id"`
	SummaryDate      time.Time `gorm:"type:date;not null;uniqueIndex:unique_elaas_summary;column:summary_date" json:"summary_date"`
	TaxType          string    `gorm:"type:varchar(50);not null;uniqueIndex:unique_elaas_summary;column:tax_type" json:"tax_type"`
	TotalAmount      float64   `gorm:"type:numeric(15,2);not null;column:total_amount" json:"total_amount"`
	TransactionCount int       `gorm:"type:integer;not null;column:transaction_count" json:"transaction_count"`
	Filename         string    `gorm:"type:varchar(255);not null;column:filename" json:"filename"`
	UploadedBy       uuid.UUID `gorm:"type:uuid;not null;column:uploaded_by" json:"uploaded_by"`
	CreatedAt        time.Time `gorm:"type:timestamptz;not null;default:now();column:created_at" json:"created_at"`
}

func (ElaasDailySummary) TableName() string { return "elaas_daily_summaries" }

// ─────────────────────────────────────────────────────────────────────────────
// DTOs
// ─────────────────────────────────────────────────────────────────────────────

type DeclareTaxRequest struct {
	BusinessRegNumber string  `json:"business_reg_number" validate:"required"`
	TaxMonth          int     `json:"tax_month" validate:"required,min=1,max=12"`
	TaxYear           int     `json:"tax_year" validate:"required"`
	MonthlyRevenue    float64 `json:"monthly_revenue" validate:"required,gte=0"`
	VolumeUnits       float64 `json:"volume_units" validate:"gte=0"`
	FormFileURL       string  `json:"form_file_url" validate:"required"`
	PayerEmail        string  `json:"payer_email" validate:"required,email"`
	PayerPhone        string  `json:"payer_phone"`
}

type DeclareTaxResponse struct {
	DeclarationID uuid.UUID `json:"declaration_id"`
	CalculatedTax float64   `json:"calculated_tax"`
	Ref1          string    `json:"ref1"`
	Ref2          string    `json:"ref2"`
	QRCodeContent string    `json:"qr_code_content"`
	PaymentStatus string    `json:"payment_status"`
}

type TaxBusinessDTO struct {
	BusinessRegNumber string  `json:"business_reg_number"`
	NameTH            string  `json:"name_th"`
	TaxType           string  `json:"tax_type"`
	TaxRate           float64 `json:"tax_rate"`
	RateUnit          string  `json:"rate_unit"`
}

// ─────────────────────────────────────────────────────────────────────────────
// Repository & UseCase Interfaces (mobile scope)
// ─────────────────────────────────────────────────────────────────────────────

// TaxNewMobileRepository defines data access methods needed by the mobile app.
type TaxNewMobileRepository interface {
	GetBusinessByRegNumber(regNumber string) (*TaxBusiness, error)
	GetActiveTaxRate(taxType string) (*TaxRate, error)
	GetLatestDeclarationVersion(regNumber string, taxType string, month, year int) (int, error)
	CreateDeclaration(declaration *TaxDeclaration) error
	GetDeclarationByID(id uuid.UUID) (*TaxDeclaration, error)
}

// TaxNewMobileUseCase defines business logic exposed to the mobile endpoints.
type TaxNewMobileUseCase interface {
	GetBusiness(regNumber string) (*TaxBusinessDTO, error)
	DeclareTax(req DeclareTaxRequest) (*DeclareTaxResponse, error)
	GetDeclaration(id uuid.UUID) (*TaxDeclaration, error)
}
