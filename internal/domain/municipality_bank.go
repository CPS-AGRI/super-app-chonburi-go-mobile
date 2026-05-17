package domain

import (
	"time"

	"github.com/google/uuid"
)

type MunicipalityBank struct {
	ID                uuid.UUID `gorm:"type:uuid;primaryKey;default:uuid_generate_v4();column:id" json:"id"`
	BankAccountNumber string    `gorm:"type:text;not null;column:bank_account_number" json:"bankAccountNumber"`
	BankAccountName   string    `gorm:"type:text;not null;column:bank_account_name" json:"bankAccountName"`
	BankType          string    `gorm:"type:text;not null;column:bank_type" json:"bankType"`
	BankName          string    `gorm:"type:text;not null;column:bank_name" json:"bankName"`
	BankQrCodeUrl     string    `gorm:"type:text;column:bank_qr_code_url" json:"bankQrCodeUrl"`
	BankStatus        string    `gorm:"type:text;not null;default:'active';column:bank_status" json:"bankStatus"`
	MunicipalityId    uuid.UUID `gorm:"type:uuid;column:municipality_id" json:"municipalityId"`
	CreatedBy         string    `gorm:"type:text;not null;default:'';column:created_by" json:"createdBy"`
	CreatedAt         time.Time `gorm:"type:timestamptz;not null;default:CURRENT_TIMESTAMP;column:created_date" json:"createdAt"`
	UpdatedBy         string    `gorm:"type:text;not null;default:'';column:updated_by" json:"updatedBy"`
	UpdatedAt         time.Time `gorm:"type:timestamptz;not null;default:CURRENT_TIMESTAMP;column:updated_date" json:"updatedAt"`
	BankBranch        string    `gorm:"type:text;column:bank_branch" json:"bankBranch"`
	Status            string    `gorm:"type:text;not null;default:'active';column:status" json:"status"`
	PromptPayNumber   string    `gorm:"type:text;column:prompt_pay_number" json:"promptPayNumber"`
}

func (MunicipalityBank) TableName() string {
	return "municipality_bank_detail"
}

type MunicipalityBankRepository interface {
	GetActive() (*MunicipalityBank, error)
}

type MunicipalityBankUseCase interface {
	GetActiveBank() (*MunicipalityBank, error)
}
