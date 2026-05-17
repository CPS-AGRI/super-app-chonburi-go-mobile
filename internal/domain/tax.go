/*
	WARNING: This file is a COPY from the Admin Master (super-app-chonburi-go).
	DO NOT modify this file directly in the Mobile repository.
	Any schema changes or additions must be made in the Admin Master and synced here.
*/

package domain

import (
	"encoding/json"
	"time"

	"github.com/google/uuid"
)

const (
	TaxStatusPending   = "pending"
	TaxStatusReviewing = "reviewing"
	TaxStatusCompleted = "completed"
	TaxStatusRejected  = "rejected"
	TaxStatusOverdue   = "overdue"

	TaxLinkStatusNotLinked    = "not_linked"
	TaxLinkStatusAutoLinked   = "auto_linked"
	TaxLinkStatusManualLinked = "manual_linked"

	TaxTypeOilAndGas = "oil_and_gas_tax"
	TaxTypeTobacco   = "tobacco_tax"
	TaxTypeHotel     = "hotel_fee"
)

// module_online_tax_payments
type ModuleOnlineTaxPayment struct {
	ID             uuid.UUID `gorm:"type:uuid;primaryKey;column:id" json:"id"`
	ModuleTypeId   uuid.UUID `gorm:"type:uuid;index;not null;column:module_type_id" json:"module_type_id"`
	Name           string    `gorm:"not null;column:name" json:"name"`
	Year           string    `gorm:"not null;column:year" json:"year"`
	AdminUserId    uuid.UUID `gorm:"type:uuid;not null;column:admin_user_id" json:"admin_user_id"`
	CreatedDate    time.Time `gorm:"not null;type:timestamptz;column:created_date" json:"created_at"`
	UpdatedDate    time.Time `gorm:"not null;type:timestamptz;column:updated_date" json:"updated_at"`

	// Relations
	Informations []ModuleOnlineTaxPaymentInformation `gorm:"foreignKey:ModuleOnlineTaxPaymentId" json:"informations,omitempty"`
}

func (ModuleOnlineTaxPayment) TableName() string { return "module_online_tax_payments" }

// module_online_tax_payment_informations
type ModuleOnlineTaxPaymentInformation struct {
	ID                       uuid.UUID `gorm:"type:uuid;primaryKey;column:id" json:"id"`
	ModuleOnlineTaxPaymentId uuid.UUID `gorm:"type:uuid;index;not null;column:module_online_tax_payment_id" json:"module_online_tax_payment_id"`
	DocumentId               string    `gorm:"not null;column:document_id;index" json:"document_id"`
	Name                     string    `gorm:"not null;column:name" json:"name"`
	IdentityNumber           string    `gorm:"not null;column:identity_number;index" json:"identity_number"`
	ReferenceNumber1         string    `gorm:"not null;column:reference_number_1;index" json:"reference_number_1"`
	ReferenceNumber2         string    `gorm:"not null;column:reference_number_2;index" json:"reference_number_2"`
	Amount                   float64   `gorm:"not null;column:amount" json:"amount"`
	PaymentDueDate           time.Time `gorm:"not null;type:timestamptz;column:payment_due_date" json:"payment_due_date"`
	UserId                   *uuid.UUID `gorm:"type:uuid;index;column:user_id" json:"user_id"`
	FileUrl                  *string    `gorm:"column:file_url" json:"file_url"`
	QRCodeUrl                *string    `gorm:"column:qr_code_url" json:"qr_code_url"`
	TransactionPaymentUrl    *string    `gorm:"column:transaction_payment_url" json:"transaction_payment_url"`
	ReceiptUrl               *string    `gorm:"column:receipt_url" json:"receipt_url"`
	LinkStatus               string     `gorm:"not null;column:link_status" json:"link_status"`
	Status                   string     `gorm:"not null;column:status;index" json:"status"`
	
	// Future-ready fields for Dynamic QR
	BankTransactionID *string `gorm:"column:bank_transaction_id" json:"bank_transaction_id"`

	CreatedDate time.Time `gorm:"not null;type:timestamptz;column:created_date" json:"created_at"`
	UpdatedDate time.Time `gorm:"not null;type:timestamptz;column:updated_date" json:"updated_at"`

	// Relations
	ModuleOnlineTaxPayment *ModuleOnlineTaxPayment `gorm:"foreignKey:ModuleOnlineTaxPaymentId" json:"online_tax_payment,omitempty"`
	User                   *AppUser                `gorm:"foreignKey:UserId" json:"user,omitempty"`
}

func (ModuleOnlineTaxPaymentInformation) TableName() string {
	return "module_online_tax_payment_informations"
}

// module_online_tax_payment_logs
type ModuleOnlineTaxPaymentLog struct {
	ID                                  uuid.UUID       `gorm:"type:uuid;primaryKey;column:id" json:"id"`
	AdminUserId                         uuid.UUID       `gorm:"type:uuid;not null;column:admin_user_id" json:"admin_user_id"`
	ModuleOnlineTaxPaymentInformationId uuid.UUID       `gorm:"type:uuid;index;not null;column:module_online_tax_payment_information_id" json:"module_online_tax_payment_information_id"`
	Action                              string          `gorm:"not null;column:action" json:"action"`
	OldData                             json.RawMessage `gorm:"type:jsonb;column:old_data" json:"old_data"`
	NewData                             json.RawMessage `gorm:"type:jsonb;column:new_data" json:"new_data"`
	CreatedDate                         time.Time       `gorm:"not null;type:timestamptz;column:created_date" json:"created_at"`
}

func (ModuleOnlineTaxPaymentLog) TableName() string { return "module_online_tax_payment_logs" }

// Interfaces
type TaxQuery struct {
	PageNumber     int
	PageSize       int
	Status         []string
	LinkStatus     []string
	Year           string
	IdentityNumber string
	ModuleTypeId   string
	Keyword        string
}

type PaginatedTaxResponse struct {
	Items      []ModuleOnlineTaxPaymentInformation `json:"items"`
	TotalItems int64                               `json:"total_items"`
	PageNumber int                                 `json:"page_number"`
	PageSize   int                                 `json:"page_size"`
}

type TaxRepository interface {
	// Import related
	CreateImport(importHead *ModuleOnlineTaxPayment) error
	CreateInformation(info *ModuleOnlineTaxPaymentInformation) error
	GetImportByYearAndName(year, name string) (*ModuleOnlineTaxPayment, error)
	GetInformationByRefs(ref1, ref2 string) (*ModuleOnlineTaxPaymentInformation, error)
	
	// Query related
	GetInformationsPaginated(query TaxQuery) (*PaginatedTaxResponse, error)
	GetInformationByID(id string) (*ModuleOnlineTaxPaymentInformation, error)
	GetImportsPaginated(query TaxQuery) ([]ModuleOnlineTaxPayment, int64, error)
	
	// Update related
	UpdateInformation(info *ModuleOnlineTaxPaymentInformation) error
	
	// Logging
	CreateLog(log *ModuleOnlineTaxPaymentLog) error
	
	// Mobile specific
	GetInformationsByIdentityNumber(identityNumber string) ([]ModuleOnlineTaxPaymentInformation, error)
}

type TaxUseCase interface {
	ImportTaxRecords(importData *ModuleOnlineTaxPayment, records []ModuleOnlineTaxPaymentInformation, adminID uuid.UUID) (successCount, errorCount int, errors []string)
	GetMyTaxes(identityNumber string) ([]ModuleOnlineTaxPaymentInformation, error)
	UpdateTaxStatus(id uuid.UUID, status string, adminID uuid.UUID, receiptUrl *string) error
	LinkUser(infoID uuid.UUID, userID uuid.UUID, adminID uuid.UUID) error
}
