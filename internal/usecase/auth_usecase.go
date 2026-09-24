package usecase

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"math/big"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"super-app-chonburi-go-mobile/config"
	"super-app-chonburi-go-mobile/internal/domain"
	"super-app-chonburi-go-mobile/internal/infrastructure"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"
)

// Dedicated resilient HTTP client for third-party OAuth providers (Google, Facebook, LINE, Apple, ThaiID)
var oauthHTTPClient = &http.Client{
	Timeout: 15 * time.Second,
	Transport: &http.Transport{
		MaxIdleConns:        100,
		MaxIdleConnsPerHost: 20,
		IdleConnTimeout:     90 * time.Second,
		TLSHandshakeTimeout: 10 * time.Second,
	},
}

func verifyGoogleIDToken(idToken string, expectedClientID ...string) (googleID, email, name, picture, rawData string, err error) {
	url := fmt.Sprintf("https://oauth2.googleapis.com/tokeninfo?id_token=%s", idToken)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return "", "", "", "", "", fmt.Errorf("failed to create validation request: %w", err)
	}

	resp, err := oauthHTTPClient.Do(req)
	if err != nil {
		return "", "", "", "", "", fmt.Errorf("failed to call Google Token Info: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", "", "", "", "", errors.New("invalid google token")
	}

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", "", "", "", "", fmt.Errorf("failed to read google response: %w", err)
	}

	var payload struct {
		Aud     string `json:"aud"`
		Sub     string `json:"sub"`
		Email   string `json:"email"`
		Name    string `json:"name"`
		Picture string `json:"picture"`
	}
	if err := json.Unmarshal(bodyBytes, &payload); err != nil {
		return "", "", "", "", "", fmt.Errorf("failed to parse google token payload: %w", err)
	}

	if len(expectedClientID) > 0 && expectedClientID[0] != "" {
		if payload.Aud != expectedClientID[0] {
			return "", "", "", "", "", fmt.Errorf("google token audience mismatch: expected %s, got %s", expectedClientID[0], payload.Aud)
		}
	}

	return payload.Sub, payload.Email, payload.Name, payload.Picture, string(bodyBytes), nil
}

type otpData struct {
	Code      string    `json:"code"`
	Ref       string    `json:"ref"`
	ExpiresAt time.Time `json:"expires_at"`
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
	googleID, email, name, picture, rawDataStr, err := verifyGoogleIDToken(idToken, u.config.GoogleClientID)
	if err != nil {
		return nil, err
	}

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

			// อัปเดตข้อมูลผู้ใช้เดิมหากชื่อหรือรูปยังเป็นค่าเริ่มต้น
			userUpdated := false
			if user.Information != nil && isPlaceholderName(user.Information.Name) {
				firstName := name
				lastName := ""
				for i, char := range name {
					if char == ' ' {
						firstName = name[:i]
						lastName = name[i+1:]
						break
					}
				}
				user.Information.Name = firstName
				if lastName != "" || isPlaceholderLastName(user.Information.LastName) {
					user.Information.LastName = lastName
				}
				userUpdated = true
			}
			if picture != "" && (user.ImageProfileUrl == nil || *user.ImageProfileUrl == "") {
				user.ImageProfileUrl = &picture
				userUpdated = true
			}
			if email != "" && (user.Email == nil || *user.Email == "") {
				user.Email = &email
				if user.Information != nil {
					user.Information.Email = &email
				}
				userUpdated = true
			}
			if userUpdated {
				user.UpdatedDate = time.Now()
				if user.Information != nil {
					user.Information.UpdatedDate = time.Now()
				}
				_ = u.repo.Update(user)
			}
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

// --- Facebook Limited Login (iOS JWT Verification with In-Memory JWKS Cache) ---

type fbJWK struct {
	Kid string `json:"kid"`
	Kty string `json:"kty"`
	Alg string `json:"alg"`
	N   string `json:"n"`
	E   string `json:"e"`
}

type fbJWKS struct {
	Keys []fbJWK `json:"keys"`
}

type fbLimitedClaims struct {
	Sub     string `json:"sub"`
	Name    string `json:"name"`
	Email   string `json:"email"`
	Picture string `json:"picture"`
	jwt.RegisteredClaims
}

var (
	fbJwksMu       sync.RWMutex
	fbJwksCache    map[string]*rsa.PublicKey
	fbJwksExpireAt time.Time
)

func getFacebookPublicKey(kid string) (*rsa.PublicKey, error) {
	fbJwksMu.RLock()
	if fbJwksCache != nil && time.Now().Before(fbJwksExpireAt) {
		pubKey, ok := fbJwksCache[kid]
		fbJwksMu.RUnlock()
		if ok {
			return pubKey, nil
		}
	} else {
		fbJwksMu.RUnlock()
	}

	fbJwksMu.Lock()
	defer fbJwksMu.Unlock()

	// Double-check under lock
	if fbJwksCache != nil && time.Now().Before(fbJwksExpireAt) {
		if pubKey, ok := fbJwksCache[kid]; ok {
			return pubKey, nil
		}
	}

	resp, err := oauthHTTPClient.Get("https://limited.facebook.com/.well-known/oauth/openid/jwks/")
	if err != nil {
		return nil, fmt.Errorf("failed to fetch facebook jwks: %w", err)
	}
	defer resp.Body.Close()

	var jwks fbJWKS
	if err := json.NewDecoder(resp.Body).Decode(&jwks); err != nil {
		return nil, fmt.Errorf("failed to parse facebook jwks: %w", err)
	}

	newCache := make(map[string]*rsa.PublicKey)
	for _, k := range jwks.Keys {
		nBytes, err := base64.RawURLEncoding.DecodeString(k.N)
		if err != nil {
			continue
		}
		eBytes, err := base64.RawURLEncoding.DecodeString(k.E)
		if err != nil {
			continue
		}
		n := new(big.Int).SetBytes(nBytes)
		e := int(new(big.Int).SetBytes(eBytes).Int64())
		newCache[k.Kid] = &rsa.PublicKey{N: n, E: e}
	}

	fbJwksCache = newCache
	fbJwksExpireAt = time.Now().Add(24 * time.Hour) // Cache JWKS 24 ชม.

	pubKey, ok := fbJwksCache[kid]
	if !ok {
		return nil, fmt.Errorf("no key found for kid: %s", kid)
	}
	return pubKey, nil
}

func verifyFacebookLimitedToken(authToken string) (*fbLimitedClaims, error) {
	claims := &fbLimitedClaims{}
	token, err := jwt.ParseWithClaims(authToken, claims, func(t *jwt.Token) (interface{}, error) {
		if _, ok := t.Method.(*jwt.SigningMethodRSA); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", t.Header["alg"])
		}
		kid, _ := t.Header["kid"].(string)
		return getFacebookPublicKey(kid)
	})
	if err != nil || !token.Valid {
		return nil, fmt.Errorf("invalid facebook limited login token: %w", err)
	}
	return claims, nil
}

// --- Apple JWKS Caching (24h TTL) ---
var (
	appleJwksCache    map[string]*rsa.PublicKey
	appleJwksMu       sync.RWMutex
	appleJwksExpireAt time.Time
)

type appleClaims struct {
	Iss           string      `json:"iss"`
	Aud           string      `json:"aud"`
	Exp           int64       `json:"exp"`
	Iat           int64       `json:"iat"`
	Sub           string      `json:"sub"`
	Email         string      `json:"email"`
	EmailVerified interface{} `json:"email_verified"`
	jwt.RegisteredClaims
}

func getApplePublicKey(kid string) (*rsa.PublicKey, error) {
	appleJwksMu.RLock()
	if time.Now().Before(appleJwksExpireAt) && appleJwksCache != nil {
		pubKey, ok := appleJwksCache[kid]
		appleJwksMu.RUnlock()
		if ok {
			return pubKey, nil
		}
	} else {
		appleJwksMu.RUnlock()
	}

	appleJwksMu.Lock()
	defer appleJwksMu.Unlock()

	if time.Now().Before(appleJwksExpireAt) && appleJwksCache != nil {
		if pubKey, ok := appleJwksCache[kid]; ok {
			return pubKey, nil
		}
	}

	resp, err := oauthHTTPClient.Get("https://appleid.apple.com/auth/keys")
	if err != nil {
		return nil, fmt.Errorf("failed to fetch apple jwks: %w", err)
	}
	defer resp.Body.Close()

	var jwks struct {
		Keys []struct {
			Kid string `json:"kid"`
			N   string `json:"n"`
			E   string `json:"e"`
		} `json:"keys"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&jwks); err != nil {
		return nil, fmt.Errorf("failed to decode apple jwks: %w", err)
	}

	newCache := make(map[string]*rsa.PublicKey)
	for _, k := range jwks.Keys {
		nBytes, err := base64.RawURLEncoding.DecodeString(k.N)
		if err != nil {
			continue
		}
		eBytes, err := base64.RawURLEncoding.DecodeString(k.E)
		if err != nil {
			continue
		}
		n := new(big.Int).SetBytes(nBytes)
		e := int(new(big.Int).SetBytes(eBytes).Int64())
		newCache[k.Kid] = &rsa.PublicKey{N: n, E: e}
	}

	appleJwksCache = newCache
	appleJwksExpireAt = time.Now().Add(24 * time.Hour)

	pubKey, ok := appleJwksCache[kid]
	if !ok {
		return nil, fmt.Errorf("no apple public key found for kid: %s", kid)
	}
	return pubKey, nil
}

func verifyAppleIDToken(idToken string) (*appleClaims, error) {
	claims := &appleClaims{}
	token, err := jwt.ParseWithClaims(idToken, claims, func(t *jwt.Token) (interface{}, error) {
		if _, ok := t.Method.(*jwt.SigningMethodRSA); !ok {
			return nil, fmt.Errorf("unexpected apple signing method: %v", t.Header["alg"])
		}
		kid, _ := t.Header["kid"].(string)
		return getApplePublicKey(kid)
	})
	if err != nil || !token.Valid {
		return nil, fmt.Errorf("invalid apple identity token: %w", err)
	}

	if claims.Iss != "https://appleid.apple.com" {
		return nil, errors.New("invalid apple token issuer")
	}

	return claims, nil
}

func (u *authUseCase) LoginWithFacebookLimited(authToken string) (*domain.AuthResponse, error) {
	claims, err := verifyFacebookLimitedToken(authToken)
	if err != nil {
		return nil, err
	}

	fbUserID := claims.Sub
	fbName := claims.Name
	fbEmail := claims.Email
	fbPicture := claims.Picture

	if fbUserID == "" {
		return nil, errors.New("facebook profile id (sub) is empty")
	}

	log.Printf("[Facebook Limited] verified user id=%s name=%s", fbUserID, fbName)

	rawBytes, _ := json.Marshal(claims)
	rawDataStr := string(rawBytes)

	user, err := u.repo.GetByProviderID("facebook", fbUserID)
	if err != nil {
		if fbEmail != "" {
			user, err = u.repo.GetByEmail(fbEmail)
		}
		if err != nil || user == nil {
			newUserID := uuid.New()
			firstName := fbName
			lastName := ""
			for i, ch := range fbName {
				if ch == ' ' {
					firstName = fbName[:i]
					lastName = fbName[i+1:]
					break
				}
			}
			user = &domain.AppUser{
				ID:              newUserID,
				PhoneNumber:     "",
				ImageProfileUrl: &fbPicture,
				IsConsent:       true,
				CreatedBy:       "system",
				CreatedDate:     time.Now(),
				UpdatedBy:       "system",
				UpdatedDate:     time.Now(),
				OauthAccounts: []domain.UserOauthAccount{
					{
						ID:          uuid.New(),
						Provider:    "facebook",
						ProviderId:  fbUserID,
						Email:       fbEmail,
						DisplayName: fbName,
						AvatarUrl:   fbPicture,
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
			if fbEmail != "" {
				user.Email = &fbEmail
				user.Information.Email = &fbEmail
			}
			if err = u.repo.Create(user); err != nil {
				return nil, err
			}
		} else {
			oauthAcc := &domain.UserOauthAccount{
				ID:          uuid.New(),
				UserId:      user.ID,
				Provider:    "facebook",
				ProviderId:  fbUserID,
				Email:       fbEmail,
				DisplayName: fbName,
				AvatarUrl:   fbPicture,
				RawData:     rawDataStr,
				CreatedAt:   time.Now(),
			}
			if err = u.repo.CreateOauthAccount(oauthAcc); err != nil {
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
	if fullUser, err := u.repo.GetByID(user.ID); err == nil && fullUser != nil {
		user = fullUser
	}
	return &domain.AuthResponse{
		AccessToken:  accessTokenJWT,
		RefreshToken: refreshTokenJWT,
		User:         user,
	}, nil
}

// --- End Facebook Limited Login ---

func (u *authUseCase) LoginWithFacebook(accessToken string) (*domain.AuthResponse, error) {
	// Call Facebook Graph API to verify token and retrieve profile info
	resp, err := oauthHTTPClient.Get(fmt.Sprintf("https://graph.facebook.com/v19.0/me?fields=id,name,email,picture.type(large)&access_token=%s", accessToken))
	if err != nil {
		return nil, fmt.Errorf("failed to call facebook api: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		log.Printf("[Facebook Login] Graph API error (status %d): %s", resp.StatusCode, string(body))
		return nil, fmt.Errorf("invalid facebook token: %s", string(body))
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

			// อัปเดตข้อมูลผู้ใช้เดิมหากชื่อหรือรูปยังเป็นค่าเริ่มต้น
			userUpdated := false
			if user.Information != nil && isPlaceholderName(user.Information.Name) {
				firstName := fbProfile.Name
				lastName := ""
				for i, char := range fbProfile.Name {
					if char == ' ' {
						firstName = fbProfile.Name[:i]
						lastName = fbProfile.Name[i+1:]
						break
					}
				}
				user.Information.Name = firstName
				if lastName != "" || isPlaceholderLastName(user.Information.LastName) {
					user.Information.LastName = lastName
				}
				userUpdated = true
			}
			if picture != "" && (user.ImageProfileUrl == nil || *user.ImageProfileUrl == "") {
				user.ImageProfileUrl = &picture
				userUpdated = true
			}
			if fbProfile.Email != "" && (user.Email == nil || *user.Email == "") {
				user.Email = &fbProfile.Email
				if user.Information != nil {
					user.Information.Email = &fbProfile.Email
				}
				userUpdated = true
			}
			if userUpdated {
				user.UpdatedDate = time.Now()
				if user.Information != nil {
					user.Information.UpdatedDate = time.Now()
				}
				_ = u.repo.Update(user)
			}
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

func (u *authUseCase) LoginWithLine(codeOrToken string, redirectURI string) (*domain.AuthResponse, error) {
	lineAccessToken := codeOrToken

	// Check if codeOrToken is an authorization code that needs exchange
	if len(codeOrToken) < 60 || strings.Contains(codeOrToken, "-") == false {
		tokenData := url.Values{}
		tokenData.Set("grant_type", "authorization_code")
		tokenData.Set("code", codeOrToken)
		tokenData.Set("redirect_uri", redirectURI)
		tokenData.Set("client_id", u.config.LineChannelID)
		tokenData.Set("client_secret", u.config.LineChannelSecret)

		tokenReq, reqErr := http.NewRequest(http.MethodPost, "https://api.line.me/oauth2/v2.1/token", strings.NewReader(tokenData.Encode()))
		if reqErr == nil {
			tokenReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")
			tokenResp, err := oauthHTTPClient.Do(tokenReq)
			if err == nil && tokenResp.StatusCode == http.StatusOK {
				var tokenResult struct {
					AccessToken string `json:"access_token"`
				}
				if err := json.NewDecoder(tokenResp.Body).Decode(&tokenResult); err == nil && tokenResult.AccessToken != "" {
					lineAccessToken = tokenResult.AccessToken
				}
				tokenResp.Body.Close()
			}
		}
	}

	// 2. Fetch User Profile using Access Token
	req, err := http.NewRequest(http.MethodGet, "https://api.line.me/v2/profile", nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create profile request: %w", err)
	}
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", lineAccessToken))

	profileResp, err := oauthHTTPClient.Do(req)
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

func (u *authUseCase) LoginWithThaiID(code string, redirectURI string) (*domain.AuthResponse, error) {
	// 1. Exchange Authorization Code for Access Token (DOPA Production)
	tokenURL := "https://imauth.bora.dopa.go.th/api/v2/oauth2/token/"
	data := url.Values{}
	data.Set("grant_type", "authorization_code")
	data.Set("code", code)
	data.Set("redirect_uri", redirectURI)

	reqToken, err := http.NewRequest(http.MethodPost, tokenURL, strings.NewReader(data.Encode()))
	if err != nil {
		return nil, fmt.Errorf("failed to create token request: %w", err)
	}

	basicAuth := base64.StdEncoding.EncodeToString([]byte(u.config.ThaiIDClientID + ":" + u.config.ThaiIDClientSecret))
	reqToken.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	reqToken.Header.Set("Authorization", fmt.Sprintf("Basic %s", basicAuth))
	reqToken.Header.Set("x-api-key", u.config.ThaiIDApiKey)

	respToken, err := oauthHTTPClient.Do(reqToken)
	if err != nil {
		return nil, fmt.Errorf("failed to execute token request: %w", err)
	}
	defer respToken.Body.Close()

	if respToken.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(respToken.Body)
		bodyStr := string(bodyBytes)
		log.Printf("[ThaiID Error] Token exchange failed. Status: %d, Response: %s, RedirectURI used: %s", respToken.StatusCode, bodyStr, redirectURI)
		return nil, fmt.Errorf("token exchange failed with status %d: %s", respToken.StatusCode, bodyStr)
	}

	// DOPA returns all user profile fields directly in the token response body.
	// No separate userinfo call is required.
	tokenBodyBytes, _ := io.ReadAll(respToken.Body)

	var thaiIDProfile map[string]interface{}
	if err := json.Unmarshal(tokenBodyBytes, &thaiIDProfile); err != nil {
		return nil, fmt.Errorf("failed to parse token response: %w", err)
	}

	accessToken, _ := thaiIDProfile["access_token"].(string)
	if accessToken == "" {
		return nil, errors.New("thaiid access token is empty")
	}

	pid, _ := thaiIDProfile["pid"].(string)
	if pid == "" {
		log.Printf("[ThaiID Error] pid is empty. Full token response: %v", thaiIDProfile)
		return nil, errors.New("pid is empty in token response")
	}
	log.Printf("[ThaiID Debug] Token exchange OK. pid=%s", pid)

	// Marshal raw profile data — strip sensitive PID field before storing
	safeProfile := make(map[string]interface{})
	for k, v := range thaiIDProfile {
		if k == "pid" || k == "access_token" || k == "refresh_token" {
			continue // never persist raw PID or DOPA tokens to DB
		}
		safeProfile[k] = v
	}
	rawBytes, _ := json.Marshal(safeProfile)
	rawDataStr := string(rawBytes)


	// Acquire Redis Distributed Lock for the target PID to prevent race conditions
	if u.redisClient != nil {
		lockKey := fmt.Sprintf("lock:thaiid:%s", pid)
		acquired, err := u.redisClient.AcquireLock(context.Background(), lockKey, 10*time.Second)
		if err == nil && acquired {
			defer u.redisClient.ReleaseLock(context.Background(), lockKey)
		}
	}

	// 3. Match or register user based on HMAC hash of pid (not raw pid)
	pidHash := hmacSHA256(pid, u.config.PIDHmacSecret)
	user, err := u.repo.GetByProviderID("thaiid", pidHash)
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
		houseNumber, villageNumber, alley, road, subdistrict, district, province, postalCode := parseThaiIDAddress(thaiIDProfile)

		// Secure hash and encrypt PID — PDPA compliance
		identityHash := hmacSHA256(pid, u.config.PIDHmacSecret)
		identityEncrypted, err := encryptAES256GCM(pid, u.config.PIDEncryptionKey)
		if err != nil {
			log.Printf("[ThaiID Error] Failed to encrypt PID: %v", err)
			return nil, fmt.Errorf("failed to secure identity number: %w", err)
		}

		idCardTypeVal := 1

		user = &domain.AppUser{
			ID:              newUserID,
			PhoneNumber:     "", // OAuth registration registers phone later or leaves empty
			PhoneNumberHash: "",
			PinHash:         "",
			ImageProfileUrl: nil,
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
					ProviderId:  pidHash, // Stored as HMAC hash
					DisplayName: displayName,
					AvatarUrl:   "",
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
				VerificationStatus:      "verified",
				VerifiedDate:            func() *time.Time { t := time.Now(); return &t }(),
				HouseNumber:             houseNumber,
				VillageNumber:           villageNumber,
				Alley:                   alley,
				Road:                    road,
				Subdistrict:             subdistrict,
				District:                district,
				Province:                province,
				PostalCode:              postalCode,
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
	} else if user != nil && user.Information != nil {
		userUpdated := false
		if isPlaceholderName(user.Information.Name) {
			givenName, _ := thaiIDProfile["given_name"].(string)
			familyName, _ := thaiIDProfile["family_name"].(string)
			if givenName != "" {
				user.Information.Name = givenName
				userUpdated = true
			}
			if familyName != "" {
				user.Information.LastName = familyName
				userUpdated = true
			} else if isPlaceholderLastName(user.Information.LastName) {
				user.Information.LastName = ""
				userUpdated = true
			}
		}
		if user.Information.VerificationStatus != "verified" {
			user.Information.VerificationStatus = "verified"
			now := time.Now()
			user.Information.VerifiedDate = &now
			userUpdated = true
		}
		if userUpdated {
			user.UpdatedDate = time.Now()
			user.Information.UpdatedDate = time.Now()
			_ = u.repo.Update(user)
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
	token, err := jwt.Parse(refreshToken, func(t *jwt.Token) (interface{}, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", t.Header["alg"])
		}
		return []byte(u.config.JWTSecret), nil
	})
	if err != nil || !token.Valid {
		return nil, errors.New("invalid or expired refresh token")
	}

	claims, ok := token.Claims.(jwt.MapClaims)
	if !ok {
		return nil, errors.New("invalid token claims")
	}

	userIDStr, ok := claims["user_id"].(string)
	if !ok || userIDStr == "" {
		return nil, errors.New("user_id not found in token")
	}

	userID, err := uuid.Parse(userIDStr)
	if err != nil {
		return nil, errors.New("invalid user_id in token")
	}

	user, err := u.repo.GetByID(userID)
	if err != nil || user == nil {
		return nil, errors.New("user not found")
	}

	newAccessToken, err := u.generateAccessToken(user)
	if err != nil {
		return nil, fmt.Errorf("failed to generate access token: %w", err)
	}

	newRefreshToken, err := u.generateRefreshToken(user)
	if err != nil {
		return nil, fmt.Errorf("failed to generate refresh token: %w", err)
	}

	return &domain.AuthResponse{
		AccessToken:  newAccessToken,
		RefreshToken: newRefreshToken,
		User:         user,
	}, nil
}

func (u *authUseCase) generateAccessToken(user *domain.AppUser) (string, error) {
	email := ""
	if user.Information != nil && user.Information.Email != nil {
		email = *user.Information.Email
	}

	claims := jwt.MapClaims{
		"user_id": user.ID.String(),
		"email":   email,
		"exp":     time.Now().Add(time.Hour * 2).Unix(),
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
	// Rate limiting / Cooldown check (60s)
	cooldownKey := fmt.Sprintf("otp:cooldown:%s", phoneNumber)
	otpKey := fmt.Sprintf("otp:%s", phoneNumber)
	ctx := context.Background()

	if u.redisClient != nil {
		acquired, err := u.redisClient.AcquireLock(ctx, cooldownKey, 60*time.Second)
		if err == nil && !acquired {
			return nil, errors.New("กรุณารอ 60 วินาทีก่อนขอ OTP ใหม่อีกครั้ง (cooldown)")
		}
	}

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

	data := otpData{
		Code:      otpCode,
		Ref:       ref,
		ExpiresAt: time.Now().Add(5 * time.Minute),
	}

	// บันทึกใส่ Redis (พร้อม fallback ลง in-memory store)
	if u.redisClient != nil {
		if err := u.redisClient.SetJSON(ctx, otpKey, data, 5*time.Minute); err != nil {
			log.Printf("⚠️ [OTP Redis Warning] Failed to store OTP in Redis: %v. Falling back to in-memory store.", err)
			u.otpMu.Lock()
			u.otpStore[phoneNumber] = data
			u.otpMu.Unlock()
		}
	} else {
		u.otpMu.Lock()
		u.otpStore[phoneNumber] = data
		u.otpMu.Unlock()
	}

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
	var stored otpData
	found := false
	ctx := context.Background()
	otpKey := fmt.Sprintf("otp:%s", phoneNumber)

	if u.redisClient != nil {
		if err := u.redisClient.GetJSON(ctx, otpKey, &stored); err == nil && stored.Code != "" {
			found = true
		}
	}

	if !found {
		u.otpMu.RLock()
		s, exists := u.otpStore[phoneNumber]
		u.otpMu.RUnlock()
		if exists {
			stored = s
			found = true
		}
	}

	if !found {
		return nil, errors.New("OTP verification code not found or expired")
	}

	if time.Now().After(stored.ExpiresAt) {
		if u.redisClient != nil {
			_ = u.redisClient.Delete(ctx, otpKey)
		}
		u.otpMu.Lock()
		delete(u.otpStore, phoneNumber)
		u.otpMu.Unlock()
		return nil, errors.New("OTP code has expired")
	}

	if stored.Code != otp || stored.Ref != ref {
		return nil, errors.New("invalid OTP code or reference")
	}

	// ลบ OTP ออกจาก store เมื่อตรวจสอบผ่านสำเร็จ 100%
	if u.redisClient != nil {
		_ = u.redisClient.Delete(ctx, otpKey)
	}
	u.otpMu.Lock()
	delete(u.otpStore, phoneNumber)
	u.otpMu.Unlock()

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
			Status:             "active",
			VerificationStatus: "unverified",
			HouseNumber:        req.HouseNumber,
			BuildingName:       req.Building,
			RoomNumber:         req.RoomNo,
			Alley:              req.Soi,
			VillageNumber:      req.VillageNo,
			Road:               req.Road,
			Province:           req.Province,
			District:           req.District,
			Subdistrict:        req.SubDistrict,
			PostalCode:         func() int { p, _ := strconv.Atoi(req.PostalCode); return p }(),
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

// hmacSHA256 creates a keyed hash of value using HMAC-SHA256.
// Resistant to rainbow table attacks because the secret key is required.
// Used for PID hashing (lookup key in DB).
func hmacSHA256(value, secret string) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(value))
	return hex.EncodeToString(mac.Sum(nil))
}

func isPlaceholderName(name string) bool {
	cleanName := strings.TrimSpace(name)
	return cleanName == "" || cleanName == "ผู้ใช้ชลบุรีพลัส" || cleanName == "User" || strings.HasPrefix(cleanName, "ผู้ใช้")
}

func isPlaceholderLastName(lastName string) bool {
	cleanLast := strings.TrimSpace(lastName)
	return cleanLast == "" || strings.HasPrefix(cleanLast, "เบอร์ ") || strings.HasPrefix(cleanLast, "เบอร์")
}

type ParsedThaiAddress struct {
	HouseNumber   string
	VillageNumber string
	Alley         string
	Road          string
	Subdistrict   string
	District      string
	Province      string
	PostalCode    int
}

// parseThaiAddressString แยกที่อยู่ภาษาไทยแบบข้อความรวมออกเป็นฟิลด์มาตรฐาน
func parseThaiAddressString(raw string) ParsedThaiAddress {
	var res ParsedThaiAddress
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return res
	}

	// 1. รหัสไปรษณีย์ 5 หลัก
	rePost := regexp.MustCompile(`\b([1-9][0-9]{4})\b`)
	if match := rePost.FindStringSubmatch(raw); len(match) > 1 {
		res.PostalCode, _ = strconv.Atoi(match[1])
		raw = rePost.ReplaceAllString(raw, " ")
	}

	// 2. จังหวัด (จ. / จังหวัด / กรุงเทพฯ / กรุงเทพมหานคร / กทม)
	reProv := regexp.MustCompile(`(?:จังหวัด|จ\.)\s*([ก-๙]+)`)
	if match := reProv.FindStringSubmatch(raw); len(match) > 1 {
		res.Province = strings.TrimSpace(match[1])
		raw = reProv.ReplaceAllString(raw, " ")
	} else if strings.Contains(raw, "กรุงเทพมหานคร") {
		res.Province = "กรุงเทพมหานคร"
		raw = strings.Replace(raw, "กรุงเทพมหานคร", " ", 1)
	} else if strings.Contains(raw, "กรุงเทพฯ") {
		res.Province = "กรุงเทพมหานคร"
		raw = strings.Replace(raw, "กรุงเทพฯ", " ", 1)
	} else if strings.Contains(raw, "กทม") {
		res.Province = "กรุงเทพมหานคร"
		raw = strings.Replace(raw, "กทม", " ", 1)
	}

	// 3. อำเภอ / เขต (อ. / อำเภอ / เขต)
	reDist := regexp.MustCompile(`(?:อำเภอ|เขต|อ\.)\s*([ก-๙]+)`)
	if match := reDist.FindStringSubmatch(raw); len(match) > 1 {
		res.District = strings.TrimSpace(match[1])
		raw = reDist.ReplaceAllString(raw, " ")
	}

	// 4. ตำบล / แขวง (ต. / ตำบล / แขวง)
	reSub := regexp.MustCompile(`(?:ตำบล|แขวง|ต\.)\s*([ก-๙]+)`)
	if match := reSub.FindStringSubmatch(raw); len(match) > 1 {
		res.Subdistrict = strings.TrimSpace(match[1])
		raw = reSub.ReplaceAllString(raw, " ")
	}

	// 5. ถนน (ถ. / ถนน)
	reRoad := regexp.MustCompile(`(?:ถนน|ถ\.)\s*([ก-๙0-9/]+)`)
	if match := reRoad.FindStringSubmatch(raw); len(match) > 1 {
		res.Road = strings.TrimSpace(match[1])
		raw = reRoad.ReplaceAllString(raw, " ")
	}

	// 6. ซอย / ตรอก (ซ. / ซอย / ตรอก)
	reAlley := regexp.MustCompile(`(?:ตรอก\/ซอย|ตรอก|ซอย|ซ\.)\s*([ก-๙0-9/]+(?:\s+[0-9]+(?:\/[0-9]+)?)?)`)
	if match := reAlley.FindStringSubmatch(raw); len(match) > 1 {
		res.Alley = strings.TrimSpace(match[1])
		raw = reAlley.ReplaceAllString(raw, " ")
	}

	// 7. หมู่ที่ (ม. / หมู่ / หมู่ที่)
	reVillage := regexp.MustCompile(`(?:หมู่ที่|หมู่|ม\.)\s*([0-9]+)`)
	if match := reVillage.FindStringSubmatch(raw); len(match) > 1 {
		res.VillageNumber = strings.TrimSpace(match[1])
		raw = reVillage.ReplaceAllString(raw, " ")
	}

	// 8. บ้านเลขที่ (บ้านเลขที่ / เลขที่ หรือเศษส่วนเลขที่เหลือ)
	reHouse := regexp.MustCompile(`(?:บ้านเลขที่|เลขที่)?\s*([0-9]+(?:\/[0-9]+)?)`)
	if match := reHouse.FindStringSubmatch(raw); len(match) > 1 && match[1] != "" {
		res.HouseNumber = strings.TrimSpace(match[1])
	}

	return res
}

func parseThaiIDAddress(thaiIDProfile map[string]interface{}) (houseNumber, villageNumber, alley, road, subdistrict, district, province string, postalCode int) {
	if houseAddrMap, ok := thaiIDProfile["house_address"].(map[string]interface{}); ok {
		if rawAddr, ok := houseAddrMap["raw"].(string); ok && rawAddr != "" {
			parts := strings.Split(rawAddr, "#")
			if len(parts) > 0 {
				houseNumber = strings.TrimSpace(parts[0])
			}
			if len(parts) > 1 {
				villageNumber = strings.TrimSpace(parts[1])
			}
			if len(parts) > 2 {
				alley = strings.TrimSpace(parts[2])
			}
			if len(parts) > 3 && parts[3] != "" {
				if alley == "" {
					alley = strings.TrimSpace(parts[3])
				} else {
					alley = alley + " " + strings.TrimSpace(parts[3])
				}
			}
			if len(parts) > 4 {
				road = strings.TrimSpace(parts[4])
			}
			if len(parts) > 5 {
				subdistrict = strings.TrimSpace(parts[5])
			}
			if len(parts) > 6 {
				district = strings.TrimSpace(parts[6])
			}
			if len(parts) > 7 {
				province = strings.TrimSpace(parts[7])
			}
		}
	}

	var combinedStr string
	if addrMap, ok := thaiIDProfile["address"].(map[string]interface{}); ok {
		if formattedAddr, ok := addrMap["formatted"].(string); ok {
			combinedStr = strings.TrimSpace(formattedAddr)
		}
	} else if addrStr, ok := thaiIDProfile["address"].(string); ok && addrStr != "" {
		combinedStr = strings.TrimSpace(addrStr)
	}

	// หากไม่มีข้อมูลแบบ raw หรือข้อมูลบางตัวว่าง และมี combinedStr หรือ subdistrict มีข้อความรวมที่อยู่
	if combinedStr != "" || strings.Contains(subdistrict, "ต.") || strings.Contains(subdistrict, "อ.") || strings.Contains(subdistrict, "จ.") || strings.Contains(subdistrict, "แขวง") || strings.Contains(subdistrict, "เขต") {
		targetStr := combinedStr
		if targetStr == "" {
			targetStr = subdistrict
		}
		parsed := parseThaiAddressString(targetStr)
		if houseNumber == "" {
			houseNumber = parsed.HouseNumber
		}
		if villageNumber == "" {
			villageNumber = parsed.VillageNumber
		}
		if alley == "" {
			alley = parsed.Alley
		}
		if road == "" {
			road = parsed.Road
		}
		if district == "" || strings.Contains(district, "อ.") {
			district = parsed.District
		}
		if province == "" || strings.Contains(province, "จ.") {
			province = parsed.Province
		}
		if postalCode == 0 {
			postalCode = parsed.PostalCode
		}
		if parsed.Subdistrict != "" {
			subdistrict = parsed.Subdistrict
		} else if strings.Contains(subdistrict, "ต.") || strings.Contains(subdistrict, "อ.") {
			subdistrict = ""
		}
	}

	return
}

// encryptAES256GCM encrypts plaintext using AES-256-GCM with a random 12-byte nonce.
// keyHex must be a 64-character hex string (32 bytes = 256-bit key).
// Returns base64(nonce + ciphertext) for storage.
func encryptAES256GCM(plaintext, keyHex string) (string, error) {
	keyBytes, err := hex.DecodeString(keyHex)
	if err != nil || len(keyBytes) != 32 {
		return "", fmt.Errorf("PID_ENCRYPTION_KEY must be a 64-char hex string (32 bytes): %w", err)
	}
	block, err := aes.NewCipher(keyBytes)
	if err != nil {
		return "", fmt.Errorf("failed to create AES cipher: %w", err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", fmt.Errorf("failed to create GCM: %w", err)
	}
	nonce := make([]byte, gcm.NonceSize()) // 12 bytes
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", fmt.Errorf("failed to generate nonce: %w", err)
	}
	ciphertext := gcm.Seal(nonce, nonce, []byte(plaintext), nil)
	return base64.StdEncoding.EncodeToString(ciphertext), nil
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

	// 2. ตรวจสอบ Token เพื่อเอา oauthID จาก Provider
	var oauthID string
	if provider == "facebook" {
		// รองรับทั้ง Limited Login JWT Token และ Standard Graph API Access Token
		if claims, err := verifyFacebookLimitedToken(idToken); err == nil && claims.Sub != "" {
			oauthID = claims.Sub
		} else {
			// Fallback: Call Facebook Graph API for Standard Access Token
			fbURL := fmt.Sprintf("https://graph.facebook.com/v19.0/me?fields=id&access_token=%s", idToken)
			fbReq, err := http.NewRequestWithContext(context.Background(), http.MethodGet, fbURL, nil)
			if err != nil {
				return nil, fmt.Errorf("failed to create facebook request: %w", err)
			}
			resp, err := oauthHTTPClient.Do(fbReq)
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
		}
	} else if provider == "line" {
		// Fetch LINE profile using idToken as access token with resilient HTTP client
		req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, "https://api.line.me/v2/profile", nil)
		if err == nil {
			req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", idToken))
			if resp, err := oauthHTTPClient.Do(req); err == nil && resp.StatusCode == http.StatusOK {
				var lineProfile struct {
					UserID string `json:"userId"`
				}
				_ = json.NewDecoder(resp.Body).Decode(&lineProfile)
				resp.Body.Close()
				oauthID = lineProfile.UserID
			}
		}
		if oauthID == "" {
			return nil, errors.New("invalid or expired line token")
		}
	} else if provider == "thaiid" {
		// idToken for thaiid is the pidHash (HMAC hash of PID) or provider_id
		oauthID = idToken
	} else {
		// Google Validation
		sub, _, _, _, _, err := verifyGoogleIDToken(idToken, u.config.GoogleClientID)
		if err != nil {
			return nil, errors.New("invalid google token")
		}
		oauthID = sub
	}

	// ค้นหาบัญชีชั่วคราวจาก oauthID หรือ tempUser ID (UUID)
	tempUser, err := u.repo.GetByProviderID(provider, oauthID)
	if err != nil {
		if parsedID, parseErr := uuid.Parse(oauthID); parseErr == nil {
			tempUser, err = u.repo.GetByID(parsedID)
		}
	}
	if err != nil || tempUser == nil {
		return nil, fmt.Errorf("temporary %s user not found", provider)
	}

	// ค้นหาว่าเบอร์โทรศัพท์นี้มีอยู่ในระบบแล้วหรือยัง (Primary Key identity by phone)
	existingUser, err := u.repo.GetByPhoneNumber(phoneNumber)
	if err != nil {
		// กรณีที่ 1: ยังไม่มีเบอร์นี้ในระบบ (อัปเดตเบอร์โทรลงบัญชีชั่วคราว)
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

		if tempUser.Information == nil {
			tempUser.Information = &domain.UserInformation{
				UserId:    tempUser.ID,
				Name:      "ผู้ใช้ชลบุรีพลัส",
				LastName:  "",
				Phone:     phoneNumber,
				Status:    "active",
				IsConsent: true,
			}
		} else {
			tempUser.Information.Phone = phoneNumber
			tempUser.Information.UpdatedDate = time.Now()
		}

		err = u.repo.Update(tempUser)
		if err != nil {
			return nil, fmt.Errorf("failed to update user: %w", err)
		}

		// Re-fetch full updated tempUser
		if fullUser, err := u.repo.GetByID(tempUser.ID); err == nil && fullUser != nil {
			tempUser = fullUser
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

	// กรณีที่ 2: มีเบอร์นี้ในระบบอยู่แล้ว (ย้าย OAuth Accounts จากบัญชีชั่วคราวมาผูกกับ existingUser)
	// หากมีการส่ง PIN มาเพื่อยืนยันตัวตน ให้ตรวจสอบ PIN ของ existingUser
	if pin != "" && existingUser.PinHash != "" {
		if err := bcrypt.CompareHashAndPassword([]byte(existingUser.PinHash), []byte(pin)); err != nil {
			return nil, errors.New("รหัส PIN ไม่ถูกต้อง")
		}
	}

	// Conflict Check: ตรวจสอบว่า existingUser เคยผูกบัญชีของ Provider นี้ด้วย ID อื่นไว้แล้วหรือไม่
	existingProfile, _ := u.repo.GetExistingSocialProfile(existingUser.ID, provider)
	if existingProfile != nil {
		for _, acc := range tempUser.OauthAccounts {
			if strings.EqualFold(acc.Provider, provider) && existingProfile.ProviderId != acc.ProviderId {
				return nil, &domain.SocialConflictError{
					Provider:            provider,
					ExistingAccountName: existingProfile.DisplayName,
					OAuthProfileID:      acc.ID.String(),
				}
			}
		}
	}

	for i := range tempUser.OauthAccounts {
		acc := tempUser.OauthAccounts[i]
		// ลบ record เก่าที่ผูกกับ tempUser ก่อน
		_ = u.repo.DeleteOauthAccount(acc.ID)
		
		// สร้าง record ใหม่ผูกกับ existingUser.ID
		newOauth := &domain.UserOauthAccount{
			ID:          uuid.New(),
			UserId:      existingUser.ID,
			Provider:    acc.Provider,
			ProviderId:  acc.ProviderId,
			Email:       acc.Email,
			DisplayName: acc.DisplayName,
			AvatarUrl:   acc.AvatarUrl,
			RawData:     acc.RawData,
			CreatedAt:   time.Now(),
		}
		if err := u.repo.CreateOauthAccount(newOauth); err != nil {
			log.Printf("[BindPhone] warning creating oauth account for existing user: %v", err)
		}
	}

	// หาก tempUser มีข้อมูลยืนยันตัวตนจาก DOPA (ThaiID) ให้อัปเดตข้อมูลใส่ existingUser.Information
	if tempUser.Information != nil {
		if existingUser.Information == nil {
			existingUser.Information = &domain.UserInformation{
				UserId: existingUser.ID,
			}
		}
		if tempUser.Information.Prefix != "" {
			existingUser.Information.Prefix = tempUser.Information.Prefix
		}
		if isPlaceholderName(existingUser.Information.Name) {
			if tempUser.Information.Name != "" {
				existingUser.Information.Name = tempUser.Information.Name
			}
			if tempUser.Information.LastName != "" {
				existingUser.Information.LastName = tempUser.Information.LastName
			} else if isPlaceholderLastName(existingUser.Information.LastName) {
				existingUser.Information.LastName = ""
			}
		} else {
			if tempUser.Information.Name != "" && existingUser.Information.Name == "" {
				existingUser.Information.Name = tempUser.Information.Name
			}
			if tempUser.Information.LastName != "" && existingUser.Information.LastName == "" {
				existingUser.Information.LastName = tempUser.Information.LastName
			}
		}
		if tempUser.ImageProfileUrl != nil && *tempUser.ImageProfileUrl != "" && (existingUser.ImageProfileUrl == nil || *existingUser.ImageProfileUrl == "") {
			existingUser.ImageProfileUrl = tempUser.ImageProfileUrl
		}
		if tempUser.Email != nil && *tempUser.Email != "" && (existingUser.Email == nil || *existingUser.Email == "") {
			existingUser.Email = tempUser.Email
			existingUser.Information.Email = tempUser.Email
		}
		if tempUser.Information.Birthday != nil {
			existingUser.Information.Birthday = tempUser.Information.Birthday
		}
		if tempUser.Information.IdentityNumberEncrypted != "" {
			existingUser.Information.IdentityNumberEncrypted = tempUser.Information.IdentityNumberEncrypted
		}
		if tempUser.Information.IdentityNumberHash != "" {
			existingUser.Information.IdentityNumberHash = tempUser.Information.IdentityNumberHash
		}
		if tempUser.Information.HouseNumber != "" {
			existingUser.Information.HouseNumber = tempUser.Information.HouseNumber
		}
		if tempUser.Information.VillageNumber != "" {
			existingUser.Information.VillageNumber = tempUser.Information.VillageNumber
		}
		if tempUser.Information.Alley != "" {
			existingUser.Information.Alley = tempUser.Information.Alley
		}
		if tempUser.Information.Road != "" {
			existingUser.Information.Road = tempUser.Information.Road
		}
		if tempUser.Information.Subdistrict != "" {
			existingUser.Information.Subdistrict = tempUser.Information.Subdistrict
		}
		if tempUser.Information.District != "" {
			existingUser.Information.District = tempUser.Information.District
		}
		if tempUser.Information.Province != "" {
			existingUser.Information.Province = tempUser.Information.Province
		}
		if tempUser.Information.IdCardType != nil {
			existingUser.Information.IdCardType = tempUser.Information.IdCardType
		}
		if tempUser.Information.IdCardExpiry != nil {
			existingUser.Information.IdCardExpiry = tempUser.Information.IdCardExpiry
		}
		if tempUser.Information.VerificationStatus == "verified" {
			existingUser.Information.VerificationStatus = "verified"
			existingUser.Information.VerifiedDate = tempUser.Information.VerifiedDate
		}
		existingUser.Information.UpdatedDate = time.Now()
		if err := u.repo.Update(existingUser); err != nil {
			log.Printf("[BindPhone] warning updating user information for existing user: %v", err)
		}
	}

	// ลบบัญชีชั่วคราวทิ้ง เพื่อป้องกันการได้บัญชีชั่วคราวค้างอยู่
	_ = u.repo.Delete(tempUser)

	// โหลดบัญชีเดิมตัวเต็มที่รวม OAuth Accounts ทั้งหมดเรียบร้อยแล้ว
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

func (u *authUseCase) bindOverride(req domain.BindOverrideRequest, provider string) (*domain.AuthResponse, error) {
	user, err := u.repo.GetByPhoneNumber(req.Phone)
	if err != nil || user == nil {
		return nil, fmt.Errorf("user with phone number not found: %w", err)
	}

	// Verify PIN for zero-trust security
	if user.PinHash == "" {
		return nil, errors.New("บัญชีนี้ยังไม่ได้ตั้งค่ารหัส PIN")
	}
	if err := bcrypt.CompareHashAndPassword([]byte(user.PinHash), []byte(req.Pin)); err != nil {
		return nil, errors.New("รหัส PIN ไม่ถูกต้อง")
	}

	oauthID, err := uuid.Parse(req.OAuthProfileID)
	if err != nil {
		return nil, errors.New("invalid oauth_profile_id format")
	}

	// Atomic transaction: detach old profile and bind new profile
	if err := u.repo.BindOverrideSocialAccount(user.ID, provider, oauthID); err != nil {
		return nil, fmt.Errorf("failed to override social binding: %w", err)
	}

	updatedUser, err := u.repo.GetByID(user.ID)
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

	log.Printf("[SECURITY AUDIT] Social account override successful: user_id=%s, provider=%s, new_oauth_id=%s", user.ID, provider, req.OAuthProfileID)

	return &domain.AuthResponse{
		AccessToken:  accessToken,
		RefreshToken: refreshToken,
		User:         updatedUser,
	}, nil
}

func (u *authUseCase) BindOverrideGoogle(req domain.BindOverrideRequest) (*domain.AuthResponse, error) {
	return u.bindOverride(req, "google")
}

func (u *authUseCase) BindOverrideFacebook(req domain.BindOverrideRequest) (*domain.AuthResponse, error) {
	return u.bindOverride(req, "facebook")
}

func (u *authUseCase) BindOverrideLine(req domain.BindOverrideRequest) (*domain.AuthResponse, error) {
	return u.bindOverride(req, "line")
}

func (u *authUseCase) BindOverrideApple(req domain.BindOverrideRequest) (*domain.AuthResponse, error) {
	return u.bindOverride(req, "apple")
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
	// 1. Exchange Authorization Code for Access Token (DOPA Production)
	tokenURL := "https://imauth.bora.dopa.go.th/api/v2/oauth2/token/"
	formData := url.Values{}
	formData.Set("grant_type", "authorization_code")
	formData.Set("code", code)
	formData.Set("redirect_uri", redirectURI)

	reqToken, err := http.NewRequestWithContext(context.Background(), http.MethodPost, tokenURL, strings.NewReader(formData.Encode()))
	if err != nil {
		return nil, fmt.Errorf("failed to create token request: %w", err)
	}
	basicAuth := base64.StdEncoding.EncodeToString([]byte(u.config.ThaiIDClientID + ":" + u.config.ThaiIDClientSecret))
	reqToken.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	reqToken.Header.Set("Authorization", fmt.Sprintf("Basic %s", basicAuth))
	reqToken.Header.Set("x-api-key", u.config.ThaiIDApiKey)

	respToken, err := oauthHTTPClient.Do(reqToken)
	if err != nil {
		return nil, fmt.Errorf("failed to execute token request: %w", err)
	}
	defer respToken.Body.Close()

	if respToken.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(respToken.Body)
		bodyStr := string(bodyBytes)
		log.Printf("[ThaiID Error] BindThaiID Token exchange failed. Status: %d, Response: %s, RedirectURI used: %s", respToken.StatusCode, bodyStr, redirectURI)
		return nil, fmt.Errorf("token exchange failed with status %d: %s", respToken.StatusCode, bodyStr)
	}

	// 2. DOPA returns all user profile fields directly in the token response body.
	// No separate userinfo call is required.
	tokenBodyBytes, _ := io.ReadAll(respToken.Body)

	var dopaProfile map[string]interface{}
	if err := json.Unmarshal(tokenBodyBytes, &dopaProfile); err != nil {
		return nil, fmt.Errorf("failed to parse token response: %w", err)
	}

	accessToken, _ := dopaProfile["access_token"].(string)
	if accessToken == "" {
		return nil, errors.New("thaiid access token is empty")
	}

	pid, _ := dopaProfile["pid"].(string)
	if pid == "" {
		log.Printf("[ThaiID Error] BindThaiID pid is empty. Full token response: %v", dopaProfile)
		return nil, errors.New("pid is empty in token response")
	}
	log.Printf("[ThaiID Debug] BindThaiID Token exchange OK. pid=%s", pid)

	// 3. ป้องกัน Race Condition ด้วย Redis Lock
	if u.redisClient != nil {
		lockKey := fmt.Sprintf("lock:thaiid:%s", pid)
		acquired, err := u.redisClient.AcquireLock(context.Background(), lockKey, 10*time.Second)
		if err == nil && acquired {
			defer u.redisClient.ReleaseLock(context.Background(), lockKey)
		}
	}

	// 4. ตรวจสอบว่า ThaiID PID นี้ถูกผูกกับบัญชีอื่นอยู่แล้วหรือไม่ (ใช้ pidHash ให้ตรงกับ LoginWithThaiID)
	pidHash := hmacSHA256(pid, u.config.PIDHmacSecret)
	if existing, err := u.repo.GetByProviderID("thaiid", pidHash); err == nil {
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

	// 6. Strip sensitive PID field before storing raw profile data
	safeProfile := make(map[string]interface{})
	for k, v := range dopaProfile {
		if k == "pid" || k == "access_token" || k == "refresh_token" {
			continue
		}
		safeProfile[k] = v
	}
	rawBytes, _ := json.Marshal(safeProfile)

	// 7. สร้าง OauthAccount ใหม่และ CreateOauthAccount
	oauthAcc := &domain.UserOauthAccount{
		ID:          uuid.New(),
		UserId:      parsedID,
		Provider:    "thaiid",
		ProviderId:  pidHash, // Stored as HMAC hash consistent with LoginWithThaiID
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

		// Hash PID using HMAC-SHA256 (consistent with LoginWithThaiID)
		user.Information.IdentityNumberHash = hmacSHA256(pid, u.config.PIDHmacSecret)
		if user.Information.IdentityNumberEncrypted == "" {
			if encrypted, err := encryptAES256GCM(pid, u.config.PIDEncryptionKey); err == nil {
				user.Information.IdentityNumberEncrypted = encrypted
			} else {
				user.Information.IdentityNumberEncrypted = "ENC_" + pid
			}
		}

		// อัปเดตชื่อ-นามสกุลทางการจาก DOPA ThaiID
		givenName, _ := dopaProfile["given_name"].(string)
		familyName, _ := dopaProfile["family_name"].(string)
		if givenName != "" {
			user.Information.Name = givenName
		}
		if familyName != "" {
			user.Information.LastName = familyName
		} else if isPlaceholderLastName(user.Information.LastName) {
			user.Information.LastName = ""
		}

		// คำนำหน้า
		title, _ := dopaProfile["titleTh"].(string)
		if title == "" {
			title, _ = dopaProfile["title"].(string)
		}
		if title != "" {
			user.Information.Prefix = title
		}

		// วันเกิด
		if birthdateStr, ok := dopaProfile["birthdate"].(string); ok && birthdateStr != "" {
			if t, err := time.Parse("2006-01-02", birthdateStr); err == nil {
				user.Information.Birthday = &t
			}
		}

		// วันหมดอายุบัตร
		if expiryStr, ok := dopaProfile["date_of_expiry"].(string); ok && expiryStr != "" {
			if t, err := time.Parse("2006-01-02", expiryStr); err == nil {
				user.Information.IdCardExpiry = &t
			}
		}

		idCardTypeVal := 1
		user.Information.IdCardType = &idCardTypeVal

		// ดึงที่อยู่ตามทะเบียนบ้านจาก ThaiID
		hNo, vNo, alley, road, subdist, dist, prov, pCode := parseThaiIDAddress(dopaProfile)
		if hNo != "" {
			user.Information.HouseNumber = hNo
		}
		if vNo != "" {
			user.Information.VillageNumber = vNo
		}
		if alley != "" {
			user.Information.Alley = alley
		}
		if road != "" {
			user.Information.Road = road
		}
		if subdist != "" {
			user.Information.Subdistrict = subdist
		}
		if dist != "" {
			user.Information.District = dist
		}
		if prov != "" {
			user.Information.Province = prov
		}
		if pCode > 0 {
			user.Information.PostalCode = pCode
		}

		user.Information.UpdatedDate = now
		user.UpdatedDate = now
		if err := u.repo.Update(user); err != nil {
			log.Printf("⚠️  BindThaiID: failed to update user information: %v", err)
		}
	}

	// 9. Reload และออก Token
	updatedUser, err := u.repo.GetByID(parsedID)
	if err != nil {
		updatedUser = user
	}

	accessToken, err = u.generateAccessToken(updatedUser)
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

func (u *authUseCase) GetSocialLinks(userID uuid.UUID) (*domain.SocialAccountsResponse, error) {
	return u.repo.GetSocialLinks(userID)
}

func (u *authUseCase) UnlinkSocial(userID uuid.UUID, provider string) error {
	return u.repo.UnlinkSocial(userID, provider)
}

func (u *authUseCase) LinkSocial(userID uuid.UUID, req domain.LinkSocialRequest) error {
	switch strings.ToLower(req.Provider) {
	case "google":
		googleID, email, name, picture, rawDataStr, err := verifyGoogleIDToken(req.IDToken, u.config.GoogleClientID)
		if err != nil {
			return errors.New("invalid google token")
		}

		account := &domain.UserOauthAccount{
			Provider:    "Google",
			ProviderId:  googleID,
			Email:       email,
			DisplayName: name,
			AvatarUrl:   picture,
			RawData:     rawDataStr,
			CreatedAt:   time.Now(),
		}
		return u.repo.LinkSocialAccount(userID, account)

	case "facebook":
		fbURL := fmt.Sprintf("https://graph.facebook.com/v19.0/me?fields=id,name,email,picture.type(large)&access_token=%s", req.AccessToken)
		fbReq, err := http.NewRequestWithContext(context.Background(), http.MethodGet, fbURL, nil)
		if err != nil {
			return fmt.Errorf("failed to create facebook request: %w", err)
		}
		resp, err := oauthHTTPClient.Do(fbReq)
		if err != nil {
			return fmt.Errorf("failed to call facebook api: %w", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			body, _ := io.ReadAll(resp.Body)
			return fmt.Errorf("invalid facebook token: %s", string(body))
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
			return fmt.Errorf("failed to parse facebook response: %w", err)
		}

		rawBytes, _ := json.Marshal(fbProfile)
		account := &domain.UserOauthAccount{
			Provider:    "Facebook",
			ProviderId:  fbProfile.ID,
			Email:       fbProfile.Email,
			DisplayName: fbProfile.Name,
			AvatarUrl:   fbProfile.Picture.Data.URL,
			RawData:     string(rawBytes),
			CreatedAt:   time.Now(),
		}
		return u.repo.LinkSocialAccount(userID, account)

	case "line":
		lineAccessToken := req.AccessToken
		if req.AuthCode != "" {
			tokenData := url.Values{}
			tokenData.Set("grant_type", "authorization_code")
			tokenData.Set("code", req.AuthCode)
			tokenData.Set("redirect_uri", req.RedirectURI)
			tokenData.Set("client_id", u.config.LineChannelID)
			tokenData.Set("client_secret", u.config.LineChannelSecret)

			tokenReq, err := http.NewRequestWithContext(context.Background(), http.MethodPost, "https://api.line.me/oauth2/v2.1/token", strings.NewReader(tokenData.Encode()))
			if err == nil {
				tokenReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")
				tokenResp, err := oauthHTTPClient.Do(tokenReq)
				if err == nil && tokenResp.StatusCode == http.StatusOK {
					var tokenResult struct {
						AccessToken string `json:"access_token"`
					}
					if err := json.NewDecoder(tokenResp.Body).Decode(&tokenResult); err == nil && tokenResult.AccessToken != "" {
						lineAccessToken = tokenResult.AccessToken
					}
					tokenResp.Body.Close()
				}
			}
		}

		lineReq, err := http.NewRequestWithContext(context.Background(), http.MethodGet, "https://api.line.me/v2/profile", nil)
		if err != nil {
			return fmt.Errorf("failed to create line profile request: %w", err)
		}
		lineReq.Header.Set("Authorization", fmt.Sprintf("Bearer %s", lineAccessToken))

		profileResp, err := oauthHTTPClient.Do(lineReq)
		if err != nil {
			return fmt.Errorf("failed to fetch line profile: %w", err)
		}
		defer profileResp.Body.Close()

		if profileResp.StatusCode != http.StatusOK {
			return fmt.Errorf("failed to verify line token (status %d)", profileResp.StatusCode)
		}

		var lineProfile struct {
			UserID      string `json:"userId"`
			DisplayName string `json:"displayName"`
			PictureURL  string `json:"pictureUrl"`
		}
		if err := json.NewDecoder(profileResp.Body).Decode(&lineProfile); err != nil {
			return fmt.Errorf("failed to parse line profile response: %w", err)
		}

		rawBytes, _ := json.Marshal(lineProfile)
		account := &domain.UserOauthAccount{
			Provider:    "Line",
			ProviderId:  lineProfile.UserID,
			DisplayName: lineProfile.DisplayName,
			AvatarUrl:   lineProfile.PictureURL,
			RawData:     string(rawBytes),
			CreatedAt:   time.Now(),
		}
		return u.repo.LinkSocialAccount(userID, account)

	case "apple":
		claims, err := verifyAppleIDToken(req.IDToken)
		if err != nil {
			return fmt.Errorf("invalid apple id token: %w", err)
		}

		account := &domain.UserOauthAccount{
			Provider:    "Apple",
			ProviderId:  claims.Sub,
			Email:       claims.Email,
			DisplayName: "Apple User",
			CreatedAt:   time.Now(),
		}
		return u.repo.LinkSocialAccount(userID, account)

	default:
		return fmt.Errorf("unsupported provider: %s", req.Provider)
	}
}

func (u *authUseCase) UpdateProfileImage(userID uuid.UUID, imageURL string) error {
	return u.repo.UpdateProfileImage(userID, imageURL)
}

