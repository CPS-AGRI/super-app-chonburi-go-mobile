package usecase

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"math/big"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"super-app-chonburi-go-mobile/config"
	"super-app-chonburi-go-mobile/internal/domain"
	"super-app-chonburi-go-mobile/internal/infrastructure"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"
	"google.golang.org/api/idtoken"
)

type otpData struct {
	Code      string
	Ref       string
	ExpiresAt time.Time
}

type authUseCase struct {
	repo        domain.AuthRepository
	config      *config.Config
	smsClient   infrastructure.SMSService
	redisClient *infrastructure.RedisClient
	otpStore    map[string]otpData
	otpMu       sync.RWMutex
}

func NewAuthUseCase(repo domain.AuthRepository, cfg *config.Config, smsClient infrastructure.SMSService, redisClient *infrastructure.RedisClient) domain.AuthUseCase {
	return &authUseCase{
		repo:        repo,
		config:      cfg,
		smsClient:   smsClient,
		redisClient: redisClient,
		otpStore:    make(map[string]otpData),
	}
}

func (u *authUseCase) LoginWithGoogle(idToken string) (*domain.AuthResponse, error) {

	payload, err := idtoken.Validate(context.Background(), idToken, "")
	if err != nil {
		return nil, errors.New("invalid google token")
	}

	email := payload.Claims["email"].(string)
	googleID := payload.Subject
	name := payload.Claims["name"].(string)
	picture := payload.Claims["picture"].(string)

	claimsBytes, _ := json.Marshal(payload.Claims)
	rawDataStr := string(claimsBytes)

	user, err := u.repo.GetByProviderID("google", googleID)
	if err != nil {
		user, err = u.repo.GetByEmail(email)
		if err != nil {
			user = &domain.AppUser{
				ID:              uuid.New(),
				PhoneNumber:     "",
				ImageProfileUrl: &picture,
				IsConsent:       true,
				CreatedBy:       "system",
				CreatedDate:     time.Now(),
				UpdatedBy:       "system",
				UpdatedDate:     time.Now(),
				OauthAccounts: []domain.UserOauthAccount{
					{
						ID:          uuid.New(),
						Provider:    "google",
						ProviderId:  googleID,
						Email:       email,
						DisplayName: name,
						AvatarUrl:   picture,
						RawData:     rawDataStr,
						CreatedAt:   time.Now(),
					},
				},
				Information: &domain.UserInformation{
					Name:      name,
					Email:     &email,
					Status:    "active",
					IsConsent: true,
					CreatedBy: "system",
				},
			}
			err = u.repo.Create(user)
			if err != nil {
				return nil, err
			}
		} else {
			oauthAcc := &domain.UserOauthAccount{
				ID:          uuid.New(),
				UserId:      user.ID,
				Provider:    "google",
				ProviderId:  googleID,
				Email:       email,
				DisplayName: name,
				AvatarUrl:   picture,
				RawData:     rawDataStr,
				CreatedAt:   time.Now(),
			}
			err = u.repo.CreateOauthAccount(oauthAcc)
			if err != nil {
				return nil, err
			}
			user.OauthAccounts = append(user.OauthAccounts, *oauthAcc)
		}
	}

	accessToken, err := u.generateAccessToken(user)
	if err != nil {
		return nil, err
	}

	refreshToken, err := u.generateRefreshToken(user)
	if err != nil {
		return nil, err
	}

	// โหลดข้อมูลเพิ่มเติม (Information, OauthAccounts) ของ User ให้ครบถ้วนก่อนส่งกลับ
	fullUser, fetchErr := u.repo.GetByID(user.ID)
	if fetchErr == nil && fullUser != nil {
		user = fullUser
	}

	return &domain.AuthResponse{
		AccessToken:  accessToken,
		RefreshToken: refreshToken,
		User:         user,
	}, nil
}

func (u *authUseCase) LoginWithFacebook(accessToken string) (*domain.AuthResponse, error) {
	// Call Facebook Graph API to verify token and retrieve profile info
	resp, err := http.Get(fmt.Sprintf("https://graph.facebook.com/v19.0/me?fields=id,name,email,picture.type(large)&access_token=%s", accessToken))
	if err != nil {
		return nil, fmt.Errorf("failed to call facebook api: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, errors.New("invalid facebook token")
	}

	var fbProfile struct {
		ID    string `json:"id"`
		Name  string `json:"name"`
		Email string `json:"email"`
		Picture struct {
			Data struct {
				URL string `json:"url"`
			} `json:"data"`
		} `json:"picture"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&fbProfile); err != nil {
		return nil, fmt.Errorf("failed to parse facebook response: %w", err)
	}

	if fbProfile.ID == "" {
		return nil, errors.New("facebook profile id is empty")
	}

	// Fetch raw data to save
	rawBytes, _ := json.Marshal(fbProfile)
	rawDataStr := string(rawBytes)

	picture := fbProfile.Picture.Data.URL

	// 1. Check if user already has this Facebook account linked
	user, err := u.repo.GetByProviderID("facebook", fbProfile.ID)
	if err != nil {
		// Not found by provider, try finding by email if it exists
		if fbProfile.Email != "" {
			user, err = u.repo.GetByEmail(fbProfile.Email)
		}

		if err != nil || user == nil {
			// Create a brand new user
			newUserID := uuid.New()
			firstName := fbProfile.Name
			lastName := ""
			for i, char := range fbProfile.Name {
				if char == ' ' {
					firstName = fbProfile.Name[:i]
					lastName = fbProfile.Name[i+1:]
					break
				}
			}

			user = &domain.AppUser{
				ID:              newUserID,
				PhoneNumber:     "",
				ImageProfileUrl: &picture,
				IsConsent:       true,
				CreatedBy:       "system",
				CreatedDate:     time.Now(),
				UpdatedBy:       "system",
				UpdatedDate:     time.Now(),
				OauthAccounts: []domain.UserOauthAccount{
					{
						ID:          uuid.New(),
						Provider:    "facebook",
						ProviderId:  fbProfile.ID,
						Email:       fbProfile.Email,
						DisplayName: fbProfile.Name,
						AvatarUrl:   picture,
						RawData:     rawDataStr,
						CreatedAt:   time.Now(),
					},
				},
				Information: &domain.UserInformation{
					UserId:             newUserID,
					Name:               firstName,
					LastName:           lastName,
					Phone:              "",
					Status:             "active",
					VerificationStatus: "unverified",
					IsConsent:          true,
					CreatedBy:          "system",
					CreatedDate:        time.Now(),
					UpdatedDate:        time.Now(),
				},
			}
			if fbProfile.Email != "" {
				user.Email = &fbProfile.Email
				user.Information.Email = &fbProfile.Email
			}

			err = u.repo.Create(user)
			if err != nil {
				return nil, err
			}
		} else {
			// Bind Facebook provider to existing user
			oauthAcc := &domain.UserOauthAccount{
				ID:          uuid.New(),
				UserId:      user.ID,
				Provider:    "facebook",
				ProviderId:  fbProfile.ID,
				Email:       fbProfile.Email,
				DisplayName: fbProfile.Name,
				AvatarUrl:   picture,
				RawData:     rawDataStr,
				CreatedAt:   time.Now(),
			}
			err = u.repo.CreateOauthAccount(oauthAcc)
			if err != nil {
				return nil, err
			}
			user.OauthAccounts = append(user.OauthAccounts, *oauthAcc)
		}
	}

	accessTokenJWT, err := u.generateAccessToken(user)
	if err != nil {
		return nil, err
	}

	refreshTokenJWT, err := u.generateRefreshToken(user)
	if err != nil {
		return nil, err
	}

	// Preload full relations
	if fullUser, err := u.repo.GetByID(user.ID); err == nil && fullUser != nil {
		user = fullUser
	}

	return &domain.AuthResponse{
		AccessToken:  accessTokenJWT,
		RefreshToken: refreshTokenJWT,
		User:         user,
	}, nil
}

func (u *authUseCase) LoginWithLine(code string, redirectURI string) (*domain.AuthResponse, error) {
	// 1. Exchange Authorization Code for Access Token
	tokenData := url.Values{}
	tokenData.Set("grant_type", "authorization_code")
	tokenData.Set("code", code)
	tokenData.Set("redirect_uri", redirectURI)
	tokenData.Set("client_id", u.config.LineChannelID)
	tokenData.Set("client_secret", u.config.LineChannelSecret)

	tokenResp, err := http.PostForm("https://api.line.me/oauth2/v2.1/token", tokenData)
	if err != nil {
		return nil, fmt.Errorf("failed to call line token api: %w", err)
	}
	defer tokenResp.Body.Close()

	if tokenResp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("invalid line auth code or secret (status %d)", tokenResp.StatusCode)
	}

	var tokenResult struct {
		AccessToken string `json:"access_token"`
	}
	if err := json.NewDecoder(tokenResp.Body).Decode(&tokenResult); err != nil {
		return nil, fmt.Errorf("failed to parse line token response: %w", err)
	}

	if tokenResult.AccessToken == "" {
		return nil, errors.New("line access token is empty")
	}

	// 2. Fetch User Profile using Access Token
	req, err := http.NewRequest(http.MethodGet, "https://api.line.me/v2/profile", nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create profile request: %w", err)
	}
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", tokenResult.AccessToken))

	profileResp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch line profile: %w", err)
	}
	defer profileResp.Body.Close()

	if profileResp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("failed to verify line token with profile api (status %d)", profileResp.StatusCode)
	}

	var lineProfile struct {
		UserID      string `json:"userId"`
		DisplayName string `json:"displayName"`
		PictureURL  string `json:"pictureUrl"`
	}
	if err := json.NewDecoder(profileResp.Body).Decode(&lineProfile); err != nil {
		return nil, fmt.Errorf("failed to parse line profile response: %w", err)
	}

	if lineProfile.UserID == "" {
		return nil, errors.New("line user id is empty")
	}

	// Fetch raw data to save
	rawBytes, _ := json.Marshal(lineProfile)
	rawDataStr := string(rawBytes)

	// 3. Match / Register user
	user, err := u.repo.GetByProviderID("line", lineProfile.UserID)
	if err != nil {
		// Create a brand new user
		newUserID := uuid.New()
		firstName := lineProfile.DisplayName
		lastName := ""
		for i, char := range lineProfile.DisplayName {
			if char == ' ' {
				firstName = lineProfile.DisplayName[:i]
				lastName = lineProfile.DisplayName[i+1:]
				break
			}
		}

		user = &domain.AppUser{
			ID:              newUserID,
			PhoneNumber:     "",
			ImageProfileUrl: &lineProfile.PictureURL,
			IsConsent:       true,
			CreatedBy:       "system",
			CreatedDate:     time.Now(),
			UpdatedBy:       "system",
			UpdatedDate:     time.Now(),
			OauthAccounts: []domain.UserOauthAccount{
				{
					ID:          uuid.New(),
					Provider:    "line",
					ProviderId:  lineProfile.UserID,
					DisplayName: lineProfile.DisplayName,
					AvatarUrl:   lineProfile.PictureURL,
					RawData:     rawDataStr,
					CreatedAt:   time.Now(),
				},
			},
			Information: &domain.UserInformation{
				UserId:             newUserID,
				Name:               firstName,
				LastName:           lastName,
				Phone:              "",
				Status:             "active",
				VerificationStatus: "unverified",
				IsConsent:          true,
				CreatedBy:          "system",
				CreatedDate:        time.Now(),
				UpdatedDate:        time.Now(),
			},
		}

		err = u.repo.Create(user)
		if err != nil {
			return nil, err
		}
	}

	accessTokenJWT, err := u.generateAccessToken(user)
	if err != nil {
		return nil, err
	}

	refreshTokenJWT, err := u.generateRefreshToken(user)
	if err != nil {
		return nil, err
	}

	return &domain.AuthResponse{
		AccessToken:  accessTokenJWT,
		RefreshToken: refreshTokenJWT,
		User:         user,
	}, nil
}

func (u *authUseCase) LoginWithThaiID(code string, redirectURI string) (*domain.AuthResponse, error) {
	// 1. Exchange Authorization Code for Access Token
	tokenURL := "https://imauthsbx.bora.dopa.go.th/api/v2/oauth2/token/" // [SANDBOX] Production: "https://imauth.bora.dopa.go.th/api/v2/oauth2/token/"
	data := url.Values{}
	data.Set("grant_type", "authorization_code")
	data.Set("code", code)
	data.Set("redirect_uri", redirectURI)

	reqToken, err := http.NewRequest(http.MethodPost, tokenURL, strings.NewReader(data.Encode()))
	if err != nil {
		return nil, fmt.Errorf("failed to create token request: %w", err)
	}

	basicAuth := base64.StdEncoding.EncodeToString([]byte(fmt.Sprintf("%s:%s", u.config.ThaiIDClientID, u.config.ThaiIDClientSecret)))
	reqToken.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	reqToken.Header.Set("Authorization", fmt.Sprintf("Basic %s", basicAuth))
	reqToken.Header.Set("x-api-key", u.config.ThaiIDApiKey)

	client := &http.Client{Timeout: 10 * time.Second}
	respToken, err := client.Do(reqToken)
	if err != nil {
		return nil, fmt.Errorf("failed to execute token request: %w", err)
	}
	defer respToken.Body.Close()

	if respToken.StatusCode != http.StatusOK {
		var errData map[string]interface{}
		_ = json.NewDecoder(respToken.Body).Decode(&errData)
		return nil, fmt.Errorf("token exchange failed with status %d: %v", respToken.StatusCode, errData)
	}

	var tokenResult struct {
		AccessToken string `json:"access_token"`
	}
	if err := json.NewDecoder(respToken.Body).Decode(&tokenResult); err != nil {
		return nil, fmt.Errorf("failed to parse token response: %w", err)
	}

	if tokenResult.AccessToken == "" {
		return nil, errors.New("thaiid access token is empty")
	}

	// 2. Fetch UserInfo profile using Access Token
	userInfoURL := "https://imauthsbx.bora.dopa.go.th/api/v2/oauth2/userinfo/" // [SANDBOX] Production: "https://imauth.bora.dopa.go.th/api/v2/oauth2/userinfo/"
	reqProfile, err := http.NewRequest(http.MethodGet, userInfoURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create userinfo request: %w", err)
	}
	reqProfile.Header.Set("Authorization", fmt.Sprintf("Bearer %s", tokenResult.AccessToken))
	reqProfile.Header.Set("x-api-key", u.config.ThaiIDApiKey)

	respProfile, err := client.Do(reqProfile)
	if err != nil {
		return nil, fmt.Errorf("failed to execute userinfo request: %w", err)
	}
	defer respProfile.Body.Close()

	if respProfile.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("failed to fetch user info (status %d)", respProfile.StatusCode)
	}

	var thaiIDProfile map[string]interface{}
	if err := json.NewDecoder(respProfile.Body).Decode(&thaiIDProfile); err != nil {
		return nil, fmt.Errorf("failed to decode userinfo response: %w", err)
	}

	pid, _ := thaiIDProfile["pid"].(string)
	if pid == "" {
		return nil, errors.New("pid is empty in userinfo response")
	}

	// Marshal raw profile data to save in UserOauthAccount
	rawBytes, _ := json.Marshal(thaiIDProfile)
	rawDataStr := string(rawBytes)

	// Acquire Redis Distributed Lock for the target PID to prevent race conditions
	if u.redisClient != nil {
		lockKey := fmt.Sprintf("lock:thaiid:%s", pid)
		acquired, err := u.redisClient.AcquireLock(context.Background(), lockKey, 10*time.Second)
		if err == nil && acquired {
			defer u.redisClient.ReleaseLock(context.Background(), lockKey)
		}
	}

	// 3. Match or register user based on pid
	user, err := u.repo.GetByProviderID("thaiid", pid)
	if err != nil {
		// Not found: Create a brand new user
		newUserID := uuid.New()
		givenName, _ := thaiIDProfile["given_name"].(string)
		familyName, _ := thaiIDProfile["family_name"].(string)
		displayName, _ := thaiIDProfile["name"].(string)
		if displayName == "" {
			displayName = givenName + " " + familyName
		}
		title, _ := thaiIDProfile["titleTh"].(string)
		if title == "" {
			title, _ = thaiIDProfile["title"].(string)
		}

		// Parse birthdate
		var birthday *time.Time
		if birthdateStr, ok := thaiIDProfile["birthdate"].(string); ok && birthdateStr != "" {
			if t, err := time.Parse("2006-01-02", birthdateStr); err == nil {
				birthday = &t
			}
		}

		// Parse card expiry
		var idCardExpiry *time.Time
		if expiryStr, ok := thaiIDProfile["date_of_expiry"].(string); ok && expiryStr != "" {
			if t, err := time.Parse("2006-01-02", expiryStr); err == nil {
				idCardExpiry = &t
			}
		}

		// Parse address from house_address or address
		var houseNumber, villageNumber, alley, road, subdistrict, district, province string
		if houseAddrMap, ok := thaiIDProfile["house_address"].(map[string]interface{}); ok {
			if rawAddr, ok := houseAddrMap["raw"].(string); ok && rawAddr != "" {
				parts := strings.Split(rawAddr, "#")
				if len(parts) > 0 { houseNumber = parts[0] }
				if len(parts) > 1 { villageNumber = parts[1] }
				if len(parts) > 2 { alley = parts[2] }
				if len(parts) > 3 && parts[3] != "" {
					if alley == "" {
						alley = parts[3]
					} else {
						alley = alley + " " + parts[3]
					}
				}
				if len(parts) > 4 { road = parts[4] }
				if len(parts) > 5 { subdistrict = parts[5] }
				if len(parts) > 6 { district = parts[6] }
				if len(parts) > 7 { province = parts[7] }
			}
		} else if addrMap, ok := thaiIDProfile["address"].(map[string]interface{}); ok {
			if formattedAddr, ok := addrMap["formatted"].(string); ok {
				subdistrict = formattedAddr
			}
		} else if addrStr, ok := thaiIDProfile["address"].(string); ok && addrStr != "" {
			subdistrict = addrStr
		}

		// Encrypt and hash identity number (PID)
		h := sha256.New()
		h.Write([]byte(pid))
		identityHash := hex.EncodeToString(h.Sum(nil))
		identityEncrypted := "ENC_" + pid

		idCardTypeVal := 1

		user = &domain.AppUser{
			ID:              newUserID,
			PhoneNumber:     "", // OAuth registration registers phone later or leaves empty
			IsConsent:       true,
			CreatedBy:       "system",
			CreatedDate:     time.Now(),
			UpdatedBy:       "system",
			UpdatedDate:     time.Now(),
			OauthAccounts: []domain.UserOauthAccount{
				{
					ID:          uuid.New(),
					UserId:      newUserID,
					Provider:    "thaiid",
					ProviderId:  pid,
					DisplayName: displayName,
					RawData:     rawDataStr,
					CreatedAt:   time.Now(),
				},
			},
			Information: &domain.UserInformation{
				UserId:                  newUserID,
				Prefix:                  title,
				Name:                    givenName,
				LastName:                familyName,
				Phone:                   "",
				Birthday:                birthday,
				IdentityNumberEncrypted: identityEncrypted,
				IdentityNumberHash:      identityHash,
				IdCardType:              &idCardTypeVal,
				IdCardExpiry:            idCardExpiry,
				Status:                  "active",
				VerificationStatus:      "verified", // Verified automatically by ThaiID
				VerifiedDate:            func() *time.Time { t := time.Now(); return &t }(),
				HouseNumber:             houseNumber,
				VillageNumber:           villageNumber,
				Alley:                   alley,
				Road:                    road,
				Subdistrict:             subdistrict,
				District:                district,
				Province:                province,
				IsConsent:               true,
				CreatedBy:               "system",
				CreatedDate:             time.Now(),
				UpdatedDate:             time.Now(),
			},
		}

		err = u.repo.Create(user)
		if err != nil {
			return nil, err
		}
	}

	// 4. Generate system access and refresh tokens
	accessTokenJWT, err := u.generateAccessToken(user)
	if err != nil {
		return nil, err
	}

	refreshTokenJWT, err := u.generateRefreshToken(user)
	if err != nil {
		return nil, err
	}

	return &domain.AuthResponse{
		AccessToken:  accessTokenJWT,
		RefreshToken: refreshTokenJWT,
		User:         user,
	}, nil
}

func (u *authUseCase) RefreshToken(refreshToken string) (*domain.AuthResponse, error) {
	return nil, errors.New("refresh token not implemented yet")
}

func (u *authUseCase) generateAccessToken(user *domain.AppUser) (string, error) {
	email := ""
	if user.Information != nil && user.Information.Email != nil {
		email = *user.Information.Email
	}

	claims := jwt.MapClaims{
		"user_id": user.ID.String(),
		"email":   email,
		"exp":     time.Now().Add(time.Hour * 24).Unix(),
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString([]byte(u.config.JWTSecret))
}

func (u *authUseCase) generateRefreshToken(user *domain.AppUser) (string, error) {
	claims := jwt.MapClaims{
		"user_id": user.ID.String(),
		"exp":     time.Now().Add(time.Hour * 24 * 30).Unix(),
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString([]byte(u.config.JWTSecret))
}

func stringPtr(s string) *string {
	return &s
}

func (u *authUseCase) RequestOTP(phoneNumber string) (*domain.OTPRequestResponse, error) {
	// เจนรหัส OTP 6 หลัก
	n, err := rand.Int(rand.Reader, big.NewInt(900000))
	if err != nil {
		return nil, fmt.Errorf("failed to generate random otp: %w", err)
	}
	otpCode := fmt.Sprintf("%06d", n.Int64()+100000)

	// เจน Reference Code 4 ตัวอักษรพิมพ์ใหญ่
	const letters = "ABCDEFGHIJKLMNOPQRSTUVWXYZ"
	refBytes := make([]byte, 4)
	for i := 0; i < 4; i++ {
		idx, err := rand.Int(rand.Reader, big.NewInt(int64(len(letters))))
		if err != nil {
			return nil, fmt.Errorf("failed to generate ref code: %w", err)
		}
		refBytes[i] = letters[idx.Int64()]
	}
	ref := string(refBytes)

	// บันทึกใส่ Memory Store
	u.otpMu.Lock()
	u.otpStore[phoneNumber] = otpData{
		Code:      otpCode,
		Ref:       ref,
		ExpiresAt: time.Now().Add(5 * time.Minute),
	}
	u.otpMu.Unlock()

	// พิมพ์รหัสออกหน้าจอ console เพื่อการทดสอบใน local dev
	log.Printf("📱 [OTP Debug] Phone: %s -> OTP: %s, Ref: %s (Expires in 5m)", phoneNumber, otpCode, ref)

	// หากมีการตั้งค่า SMS Service ให้ยิงส่ง SMS จริงไปยัง Gateway / Server ปลายทาง
	if u.smsClient != nil {
		if err := u.smsClient.SendOTP(context.Background(), phoneNumber, otpCode, ref); err != nil {
			log.Printf("⚠️ [SMS Gateway Warning] Failed to send SMS via Gateway: %v", err)
		}
	}

	return &domain.OTPRequestResponse{
		Success: true,
		Ref:     ref,
		OTP:     otpCode, // แนบไปให้หน้าบ้านหยิบใช้งานได้ทันทีในโหมด dev
	}, nil
}

func (u *authUseCase) VerifyOTP(phoneNumber, otp, ref string) (*domain.OTPVerifyResponse, error) {
	u.otpMu.Lock()
	stored, exists := u.otpStore[phoneNumber]
	if exists {
		// ลบ OTP ทิ้งทันทีเมื่อนำมาตรวจสอบ เพื่อป้องกัน replay attacks
		delete(u.otpStore, phoneNumber)
	}
	u.otpMu.Unlock()

	if !exists {
		return nil, errors.New("OTP verification code not found or expired")
	}

	if time.Now().After(stored.ExpiresAt) {
		return nil, errors.New("OTP code has expired")
	}

	if stored.Code != otp || stored.Ref != ref {
		return nil, errors.New("invalid OTP code or reference")
	}

	// ตรวจสอบสถานะการมีอยู่ของผู้ใช้งานด้วยเบอร์โทรศัพท์
	_, err := u.repo.GetByPhoneNumber(phoneNumber)
	if err != nil {
		// ไม่มีผู้ใช้ในระบบ -> คืนค่า false เพื่อให้ข้ามไปตั้ง PIN และลงทะเบียน
		// ทำการออก temp_token (JWT) เพื่อป้องกันการสวมสิทธิ์ในหน้า register
		token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
			"phone_number": phoneNumber,
			"exp":          time.Now().Add(15 * time.Minute).Unix(),
			"purpose":      "registration",
		})
		tempToken, err := token.SignedString([]byte(u.config.JWTSecret))
		if err != nil {
			return nil, err
		}

		return &domain.OTPVerifyResponse{
			Success:      true,
			IsRegistered: false,
			TempToken:    tempToken,
		}, nil
	}

	// มีผู้ใช้ในระบบแล้ว -> ส่งผลลัพธ์กลับเพื่อให้ไปหน้ากรอก PIN
	return &domain.OTPVerifyResponse{
		Success:      true,
		IsRegistered: true,
	}, nil
}

func (u *authUseCase) Register(req domain.RegisterRequest) (*domain.AuthResponse, error) {
	// Parse tempToken และทำการตรวจสอบ
	parsedToken, err := jwt.Parse(req.TempToken, func(token *jwt.Token) (interface{}, error) {
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
		}
		return []byte(u.config.JWTSecret), nil
	})

	if err != nil || !parsedToken.Valid {
		return nil, errors.New("invalid or expired temporary registration token")
	}

	claims, ok := parsedToken.Claims.(jwt.MapClaims)
	if !ok {
		return nil, errors.New("invalid token claims")
	}

	// ตรวจสอบ purpose
	if purpose, ok := claims["purpose"].(string); !ok || purpose != "registration" {
		return nil, errors.New("invalid token purpose")
	}

	phoneNumber, ok := claims["phone_number"].(string)
	if !ok || phoneNumber == "" {
		return nil, errors.New("phone number not found in token")
	}

	// เช็คซ้ำอีกครั้งว่าเบอร์มีอยู่ในระบบแล้วหรือไม่
	if _, err := u.repo.GetByPhoneNumber(phoneNumber); err == nil {
		return nil, errors.New("phone number is already registered")
	}

	// Validate SHA-256 Hash format
	isValidSHA256 := func(s string) bool {
		if len(s) != 64 {
			return false
		}
		for _, c := range s {
			if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F')) {
				return false
			}
		}
		return true
	}

	if req.IDCardHash != "" && !isValidSHA256(req.IDCardHash) {
		return nil, errors.New("invalid id_card_hash format")
	}
	if req.LaserIDHash != "" && !isValidSHA256(req.LaserIDHash) {
		return nil, errors.New("invalid laser_id_hash format")
	}

	// เข้ารหัส PIN
	hashedPin, err := bcrypt.GenerateFromPassword([]byte(req.Pin), bcrypt.DefaultCost)
	if err != nil {
		return nil, fmt.Errorf("failed to hash pin: %w", err)
	}

	// แปลง วันเกิด (ISO YYYY-MM-DD -> time.Time)
	var birthday *time.Time
	if req.Birthday != "" {
		if t, err := time.Parse("2006-01-02", req.Birthday); err == nil {
			birthday = &t
		}
	}

	name := req.FirstName
	if name == "" {
		name = "ผู้ใช้ชลบุรีพลัส"
	}
	lastName := req.LastName
	if lastName == "" {
		lastName = fmt.Sprintf("เบอร์ %s", phoneNumber[len(phoneNumber)-4:])
	}

	userID := uuid.New()
	user := &domain.AppUser{
		ID:              userID,
		PhoneNumber:     phoneNumber,
		PhoneNumberHash: hashValue(phoneNumber),
		PinHash:         string(hashedPin),
		IsConsent:       true,
		CreatedBy:       "registration_flow",
		CreatedDate:     time.Now(),
		UpdatedBy:       "registration_flow",
		UpdatedDate:     time.Now(),
		Information: &domain.UserInformation{
			UserId:             userID,
			Prefix:             req.Prefix,
			Name:               name,
			LastName:           lastName,
			Phone:              phoneNumber,
			Birthday:           birthday,
			IdentityNumberHash: req.IDCardHash,
			LaserIdHash:        req.LaserIDHash,
			Status:             "active",
			VerificationStatus: "unverified",
			BuildingName:       req.Building,
			RoomNumber:         req.RoomNo,
			Alley:              req.Soi,
			VillageNumber:      req.VillageNo,
			Road:               req.Road,
			Province:           req.Province,
			District:           req.District,
			Subdistrict:        req.SubDistrict,
			IsConsent:          true,
			CreatedBy:          "registration_flow",
			CreatedDate:        time.Now(),
			UpdatedDate:        time.Now(),
		},
	}

	if req.Email != "" {
		user.Email = &req.Email
		user.Information.Email = &req.Email
	}

	// สร้างผู้ใช้ในฐานข้อมูล
	if err := u.repo.Create(user); err != nil {
		return nil, fmt.Errorf("failed to create user: %w", err)
	}

	accessToken, err := u.generateAccessToken(user)
	if err != nil {
		return nil, err
	}

	refreshToken, err := u.generateRefreshToken(user)
	if err != nil {
		return nil, err
	}

	return &domain.AuthResponse{
		AccessToken:  accessToken,
		RefreshToken: refreshToken,
		User:         user,
	}, nil
}

func (u *authUseCase) LoginWithPin(phoneNumber, pin string) (*domain.AuthResponse, error) {
	user, err := u.repo.GetByPhoneNumber(phoneNumber)
	if err != nil {
		return nil, errors.New("invalid phone number or PIN")
	}

	// ตรวจสอบ PIN
	err = bcrypt.CompareHashAndPassword([]byte(user.PinHash), []byte(pin))
	if err != nil {
		return nil, errors.New("invalid phone number or PIN")
	}

	accessToken, err := u.generateAccessToken(user)
	if err != nil {
		return nil, err
	}

	refreshToken, err := u.generateRefreshToken(user)
	if err != nil {
		return nil, err
	}

	return &domain.AuthResponse{
		AccessToken:  accessToken,
		RefreshToken: refreshToken,
		User:         user,
	}, nil
}

func hashValue(v string) string {
	h := sha256.New()
	h.Write([]byte(v))
	return hex.EncodeToString(h.Sum(nil))
}

func (u *authUseCase) BindPhone(provider, idToken, phoneNumber, otp, ref, pin string) (*domain.AuthResponse, error) {
	// 1. ตรวจสอบรหัส OTP และ Ref Code (หากมีการส่งมาตรวจสอบ)
	if otp != "" && ref != "" {
		u.otpMu.Lock()
		stored, exists := u.otpStore[phoneNumber]
		if exists {
			delete(u.otpStore, phoneNumber)
		}
		u.otpMu.Unlock()

		if !exists {
			return nil, errors.New("OTP verification code not found or expired")
		}

		if time.Now().After(stored.ExpiresAt) {
			return nil, errors.New("OTP code has expired")
		}

		if stored.Code != otp || stored.Ref != ref {
			return nil, errors.New("invalid OTP code or reference")
		}
	}

	// 2. ตรวจสอบ JWT Token เพื่อเอา oauthID จาก Provider
	var oauthID string
	if provider == "facebook" {
		// Call Facebook Graph API to get Facebook ID
		resp, err := http.Get(fmt.Sprintf("https://graph.facebook.com/v19.0/me?fields=id&access_token=%s", idToken))
		if err != nil {
			return nil, fmt.Errorf("failed to call facebook api: %w", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			return nil, errors.New("invalid facebook token")
		}

		var fbProfile struct {
			ID string `json:"id"`
		}
		if err := json.NewDecoder(resp.Body).Decode(&fbProfile); err != nil {
			return nil, fmt.Errorf("failed to parse facebook response: %w", err)
		}
		if fbProfile.ID == "" {
			return nil, errors.New("facebook profile id is empty")
		}
		oauthID = fbProfile.ID
	} else {
		// Google Validation
		payload, err := idtoken.Validate(context.Background(), idToken, "")
		if err != nil {
			return nil, errors.New("invalid google token")
		}
		oauthID = payload.Subject
	}

	// ค้นหาบัญชีชั่วคราวจาก oauthID
	tempUser, err := u.repo.GetByProviderID(provider, oauthID)
	if err != nil {
		return nil, fmt.Errorf("temporary %s user not found", provider)
	}

	// ค้นหาว่าเบอร์โทรศัพท์นี้มีอยู่ในระบบแล้วหรือยัง
	existingUser, err := u.repo.GetByPhoneNumber(phoneNumber)
	if err != nil {
		// กรณีที่ 1: ยังไม่มีเบอร์นี้ในระบบ (อัปเดตลงบัญชีชั่วคราว)
		tempUser.PhoneNumber = phoneNumber
		tempUser.PhoneNumberHash = hashValue(phoneNumber)
		tempUser.UpdatedDate = time.Now()

		if pin != "" {
			hashedPin, err := bcrypt.GenerateFromPassword([]byte(pin), bcrypt.DefaultCost)
			if err != nil {
				return nil, fmt.Errorf("failed to hash pin: %w", err)
			}
			tempUser.PinHash = string(hashedPin)
		}

		if tempUser.Information != nil {
			tempUser.Information.Name = "ผู้ใช้ชลบุรีพลัส"
			tempUser.Information.LastName = fmt.Sprintf("เบอร์ %s", phoneNumber[len(phoneNumber)-4:])
			tempUser.Information.Phone = phoneNumber
			tempUser.Information.UpdatedDate = time.Now()
		}

		err = u.repo.Update(tempUser)
		if err != nil {
			return nil, fmt.Errorf("failed to update user: %w", err)
		}

		accessToken, err := u.generateAccessToken(tempUser)
		if err != nil {
			return nil, err
		}

		refreshToken, err := u.generateRefreshToken(tempUser)
		if err != nil {
			return nil, err
		}

		return &domain.AuthResponse{
			AccessToken:  accessToken,
			RefreshToken: refreshToken,
			User:         tempUser,
		}, nil
	}

	// ผูก Oauth Accounts ทั้งหมดจากบัญชีชั่วคราวเข้ากับบัญชีเดิม
	for i := range tempUser.OauthAccounts {
		acc := tempUser.OauthAccounts[i]
		acc.UserId = existingUser.ID
		if err := u.repo.UpdateOauthAccount(&acc); err != nil {
			return nil, fmt.Errorf("failed to update oauth account: %w", err)
		}
	}

	// ลบบัญชีชั่วคราวออกเพื่อป้องกันการล็อกอินแล้วยังได้บัญชีชั่วคราว
	err = u.repo.Delete(tempUser)
	if err != nil {
		return nil, fmt.Errorf("failed to delete temporary user: %w", err)
	}

	// โหลดบัญชีเดิมตัวล่าสุดที่รวมบัญชีเรียบร้อยแล้ว
	updatedExistingUser, err := u.repo.GetByID(existingUser.ID)
	if err != nil {
		updatedExistingUser = existingUser
	}

	accessToken, err := u.generateAccessToken(updatedExistingUser)
	if err != nil {
		return nil, err
	}

	refreshToken, err := u.generateRefreshToken(updatedExistingUser)
	if err != nil {
		return nil, err
	}

	return &domain.AuthResponse{
		AccessToken:  accessToken,
		RefreshToken: refreshToken,
		User:         updatedExistingUser,
	}, nil
}

func (u *authUseCase) CheckPhone(phoneNumber string) (bool, error) {
	_, err := u.repo.GetByPhoneNumber(phoneNumber)
	if err != nil {
		return false, nil
	}
	return true, nil
}

// BindThaiID ผูกบัญชีที่ login อยู่แล้วกับ ThaiID (DOPA) OAuth
// ขั้นตอน: Exchange code → Fetch DOPA userinfo → CreateOauthAccount → Update UserInformation
func (u *authUseCase) BindThaiID(userID string, code string, redirectURI string) (*domain.AuthResponse, error) {
	// 1. Exchange Authorization Code for Access Token (DOPA Sandbox)
	tokenURL := "https://imauthsbx.bora.dopa.go.th/api/v2/oauth2/token/" // [SANDBOX] Production: "https://imauth.bora.dopa.go.th/api/v2/oauth2/token/"
	formData := url.Values{}
	formData.Set("grant_type", "authorization_code")
	formData.Set("code", code)
	formData.Set("redirect_uri", redirectURI)

	reqToken, err := http.NewRequest(http.MethodPost, tokenURL, strings.NewReader(formData.Encode()))
	if err != nil {
		return nil, fmt.Errorf("failed to create token request: %w", err)
	}
	basicAuth := base64.StdEncoding.EncodeToString([]byte(fmt.Sprintf("%s:%s", u.config.ThaiIDClientID, u.config.ThaiIDClientSecret)))
	reqToken.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	reqToken.Header.Set("Authorization", fmt.Sprintf("Basic %s", basicAuth))
	reqToken.Header.Set("x-api-key", u.config.ThaiIDApiKey)

	httpClient := &http.Client{Timeout: 10 * time.Second}
	respToken, err := httpClient.Do(reqToken)
	if err != nil {
		return nil, fmt.Errorf("failed to execute token request: %w", err)
	}
	defer respToken.Body.Close()

	if respToken.StatusCode != http.StatusOK {
		var errData map[string]interface{}
		_ = json.NewDecoder(respToken.Body).Decode(&errData)
		return nil, fmt.Errorf("token exchange failed with status %d: %v", respToken.StatusCode, errData)
	}

	var tokenResult struct {
		AccessToken string `json:"access_token"`
	}
	if err := json.NewDecoder(respToken.Body).Decode(&tokenResult); err != nil {
		return nil, fmt.Errorf("failed to parse token response: %w", err)
	}
	if tokenResult.AccessToken == "" {
		return nil, errors.New("thaiid access token is empty")
	}

	// 2. Fetch DOPA UserInfo
	userInfoURL := "https://imauthsbx.bora.dopa.go.th/api/v2/oauth2/userinfo/" // [SANDBOX] Production: "https://imauth.bora.dopa.go.th/api/v2/oauth2/userinfo/"
	reqProfile, err := http.NewRequest(http.MethodGet, userInfoURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create userinfo request: %w", err)
	}
	reqProfile.Header.Set("Authorization", fmt.Sprintf("Bearer %s", tokenResult.AccessToken))
	reqProfile.Header.Set("x-api-key", u.config.ThaiIDApiKey)

	respProfile, err := httpClient.Do(reqProfile)
	if err != nil {
		return nil, fmt.Errorf("failed to execute userinfo request: %w", err)
	}
	defer respProfile.Body.Close()
	if respProfile.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("failed to fetch userinfo (status %d)", respProfile.StatusCode)
	}

	var dopaProfile map[string]interface{}
	if err := json.NewDecoder(respProfile.Body).Decode(&dopaProfile); err != nil {
		return nil, fmt.Errorf("failed to decode userinfo: %w", err)
	}

	pid, _ := dopaProfile["pid"].(string)
	if pid == "" {
		return nil, errors.New("pid is empty in DOPA userinfo response")
	}

	// 3. ป้องกัน Race Condition ด้วย Redis Lock
	if u.redisClient != nil {
		lockKey := fmt.Sprintf("lock:thaiid:%s", pid)
		acquired, err := u.redisClient.AcquireLock(context.Background(), lockKey, 10*time.Second)
		if err == nil && acquired {
			defer u.redisClient.ReleaseLock(context.Background(), lockKey)
		}
	}

	// 4. ตรวจสอบว่า ThaiID PID นี้ถูกผูกกับบัญชีอื่นอยู่แล้วหรือไม่
	if existing, err := u.repo.GetByProviderID("thaiid", pid); err == nil {
		if existing.ID.String() != userID {
			return nil, errors.New("ThaiID นี้ถูกผูกกับบัญชีอื่นอยู่แล้ว")
		}
		// ผูกอยู่แล้วกับบัญชีนี้ — คืน token ปัจจุบันได้เลย
		accessToken, _ := u.generateAccessToken(existing)
		refreshToken, _ := u.generateRefreshToken(existing)
		return &domain.AuthResponse{AccessToken: accessToken, RefreshToken: refreshToken, User: existing}, nil
	}

	// 5. โหลด User ปัจจุบัน
	parsedID, err := uuid.Parse(userID)
	if err != nil {
		return nil, fmt.Errorf("invalid user_id format: %w", err)
	}
	user, err := u.repo.GetByID(parsedID)
	if err != nil {
		return nil, fmt.Errorf("user not found: %w", err)
	}

	// 6. Marshal DOPA raw data
	rawBytes, _ := json.Marshal(dopaProfile)

	// 7. สร้าง OauthAccount ใหม่และ CreateOauthAccount
	oauthAcc := &domain.UserOauthAccount{
		ID:          uuid.New(),
		UserId:      parsedID,
		Provider:    "thaiid",
		ProviderId:  pid,
		DisplayName: func() string {
			if n, ok := dopaProfile["name"].(string); ok && n != "" {
				return n
			}
			given, _ := dopaProfile["given_name"].(string)
			family, _ := dopaProfile["family_name"].(string)
			return given + " " + family
		}(),
		RawData:   string(rawBytes),
		CreatedAt: time.Now(),
	}
	if err := u.repo.CreateOauthAccount(oauthAcc); err != nil {
		return nil, fmt.Errorf("failed to create thaiid oauth account: %w", err)
	}

	// 8. อัปเดต UserInformation ด้วยข้อมูล DOPA (ถ้ายังไม่มี) และ set verified
	if user.Information != nil {
		now := time.Now()
		user.Information.VerificationStatus = "verified"
		user.Information.VerifiedDate = &now

		// Hash PID และเก็บเป็น encrypted
		h := sha256.New()
		h.Write([]byte(pid))
		user.Information.IdentityNumberHash = hex.EncodeToString(h.Sum(nil))
		if user.Information.IdentityNumberEncrypted == "" {
			user.Information.IdentityNumberEncrypted = "ENC_" + pid
		}

		// เติมชื่อ-นามสกุลถ้ายังว่าง
		if givenName, ok := dopaProfile["given_name"].(string); ok && givenName != "" && user.Information.Name == "" {
			user.Information.Name = givenName
		}
		if familyName, ok := dopaProfile["family_name"].(string); ok && familyName != "" && user.Information.LastName == "" {
			user.Information.LastName = familyName
		}
		if title, ok := dopaProfile["titleTh"].(string); ok && title != "" && user.Information.Prefix == "" {
			user.Information.Prefix = title
		}
		if birthdateStr, ok := dopaProfile["birthdate"].(string); ok && birthdateStr != "" && user.Information.Birthday == nil {
			if t, err := time.Parse("2006-01-02", birthdateStr); err == nil {
				user.Information.Birthday = &t
			}
		}

		user.Information.UpdatedDate = now
		if err := u.repo.Update(user); err != nil {
			log.Printf("⚠️  BindThaiID: failed to update user information: %v", err)
		}
	}

	// 9. Reload และออก Token
	updatedUser, err := u.repo.GetByID(parsedID)
	if err != nil {
		updatedUser = user
	}

	accessToken, err := u.generateAccessToken(updatedUser)
	if err != nil {
		return nil, err
	}
	refreshToken, err := u.generateRefreshToken(updatedUser)
	if err != nil {
		return nil, err
	}

	return &domain.AuthResponse{
		AccessToken:  accessToken,
		RefreshToken: refreshToken,
		User:         updatedUser,
	}, nil
}
