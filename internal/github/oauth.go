package githubclient

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// OAuthConfig holds GitHub OAuth application credentials.
type OAuthConfig struct {
	ClientID     string
	ClientSecret string
	CallbackURL  string
	JWTSecret    string
}

// OAuthStateClaims is the JWT payload embedded in the OAuth state parameter.
// It ties the OAuth flow back to the Telegram chat that initiated it.
type OAuthStateClaims struct {
	TelegramChatID int64 `json:"telegram_chat_id"`
	jwt.RegisteredClaims
}

// GenerateAuthURL returns the GitHub OAuth authorization URL.
// The state parameter is a signed JWT containing the Telegram chat ID so
// the callback handler knows which user to link the GitHub account to.
func (o *OAuthConfig) GenerateAuthURL(telegramChatID int64) (string, error) {
	claims := OAuthStateClaims{
		TelegramChatID: telegramChatID,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(10 * time.Minute)),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	state, err := token.SignedString([]byte(o.JWTSecret))
	if err != nil {
		return "", fmt.Errorf("signing state token: %w", err)
	}

	params := url.Values{}
	params.Set("client_id", o.ClientID)
	params.Set("redirect_uri", o.CallbackURL)
	params.Set("scope", "read:user user:email")
	params.Set("state", state)

	return "https://github.com/login/oauth/authorize?" + params.Encode(), nil
}

// ValidateState parses and verifies the OAuth state JWT.
// Returns the Telegram chat ID embedded in the token.
func (o *OAuthConfig) ValidateState(state string) (int64, error) {
	token, err := jwt.ParseWithClaims(state, &OAuthStateClaims{}, func(t *jwt.Token) (interface{}, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", t.Header["alg"])
		}
		return []byte(o.JWTSecret), nil
	})
	if err != nil {
		return 0, fmt.Errorf("parsing state token: %w", err)
	}

	claims, ok := token.Claims.(*OAuthStateClaims)
	if !ok || !token.Valid {
		return 0, fmt.Errorf("invalid state token")
	}

	return claims.TelegramChatID, nil
}

// ExchangeCode exchanges a GitHub OAuth code for an access token.
func (o *OAuthConfig) ExchangeCode(ctx context.Context, code string) (string, error) {
	body := url.Values{}
	body.Set("client_id", o.ClientID)
	body.Set("client_secret", o.ClientSecret)
	body.Set("code", code)
	body.Set("redirect_uri", o.CallbackURL)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		"https://github.com/login/oauth/access_token",
		strings.NewReader(body.Encode()),
	)
	if err != nil {
		return "", fmt.Errorf("creating token request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")

	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("exchanging code: %w", err)
	}
	defer resp.Body.Close()

	var result struct {
		AccessToken string `json:"access_token"`
		TokenType   string `json:"token_type"`
		Error       string `json:"error"`
		ErrorDesc   string `json:"error_description"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", fmt.Errorf("decoding token response: %w", err)
	}
	if result.Error != "" {
		return "", fmt.Errorf("GitHub OAuth error: %s — %s", result.Error, result.ErrorDesc)
	}
	if result.AccessToken == "" {
		return "", fmt.Errorf("GitHub returned empty access token")
	}

	return result.AccessToken, nil
}
