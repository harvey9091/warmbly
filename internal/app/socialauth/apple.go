package socialauth

import (
	"crypto/ecdsa"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

const (
	validationEndpoint = "https://appleid.apple.com/auth/token"
	appleAudience      = "https://appleid.apple.com"
)

var (
	ErrorResponseInvalidGrant      = appleErrorResponseType("invalid_grant")
	ErrorResponseInvalidScope      = appleErrorResponseType("invalid_scope")
	ErrorResponseUnsupportedGrant  = appleErrorResponseType("unsupported_grant_type")
	ErrorResponseUnauthorizedClient = appleErrorResponseType("unauthorized_client")
	ErrorResponseInvalidClient     = appleErrorResponseType("invalid_client")
	ErrorResponseInvalidRequest    = appleErrorResponseType("invalid_request")
)

type appleErrorResponseType string

func (e appleErrorResponseType) Error() string {
	return string(e)
}

type AppleAuth interface {
	ValidateCode(code string) (*TokenResponse, error)
	ValidateCodeWithRedirectURI(code, redirectURI string) (*TokenResponse, error)
	ValidateRefreshToken(refreshToken string) (*TokenResponse, error)
}

type appleAuth struct {
	AppID      string
	TeamID     string
	KeyID      string
	KeyContent []byte
	httpClient *http.Client
}

type TokenResponse struct {
	AccessToken  string `json:"access_token"`
	ExpiresIn    int    `json:"expires_in"`
	IDToken      string `json:"id_token"`
	RefreshToken string `json:"refresh_token"`
	TokenType    string `json:"token_type"`
}

type appleErrorResponseBody struct {
	Error string `json:"error"`
}

func NewB64(appID, teamID, keyID, b64 string) (AppleAuth, error) {
	keyContent, err := base64.StdEncoding.DecodeString(b64)
	if err != nil {
		return nil, err
	}

	return &appleAuth{
		KeyID:      keyID,
		TeamID:     teamID,
		AppID:      appID,
		KeyContent: keyContent,
		httpClient: &http.Client{Timeout: 10 * time.Second},
	}, nil
}

func (a *appleAuth) parsePrivateKey() (*ecdsa.PrivateKey, error) {
	block, _ := pem.Decode(a.KeyContent)
	if block == nil {
		return nil, errors.New("empty block after decoding")
	}

	privateKey, err := x509.ParsePKCS8PrivateKey(block.Bytes)
	if err != nil {
		return nil, err
	}
	return privateKey.(*ecdsa.PrivateKey), nil
}

func (a *appleAuth) clientSecret() (string, error) {
	privateKey, err := a.parsePrivateKey()
	if err != nil {
		return "", err
	}

	now := time.Now()
	claims := jwt.RegisteredClaims{
		IssuedAt:  jwt.NewNumericDate(now),
		ExpiresAt: jwt.NewNumericDate(now.Add(time.Second * 15776999)),
		Issuer:    a.TeamID,
		Subject:   a.AppID,
		Audience:  jwt.ClaimStrings{appleAudience},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodES256, &claims)
	token.Header["alg"] = "ES256"
	token.Header["kid"] = a.KeyID

	return token.SignedString(privateKey)
}

func (a *appleAuth) ValidateCode(code string) (*TokenResponse, error) {
	clientSecret, err := a.clientSecret()
	if err != nil {
		return nil, err
	}
	return a.validateCode(clientSecret, code)
}

func (a *appleAuth) ValidateCodeWithRedirectURI(code, redirectURI string) (*TokenResponse, error) {
	clientSecret, err := a.clientSecret()
	if err != nil {
		return nil, err
	}
	return a.validateCodeWithRedirectURI(clientSecret, code, redirectURI)
}

func (a *appleAuth) ValidateRefreshToken(refreshToken string) (*TokenResponse, error) {
	clientSecret, err := a.clientSecret()
	if err != nil {
		return nil, err
	}
	return a.validateRefreshToken(clientSecret, refreshToken)
}

func (a *appleAuth) validateCode(clientSecret, code string) (*TokenResponse, error) {
	formQuery := make(url.Values)
	formQuery.Add("client_id", a.AppID)
	formQuery.Add("client_secret", clientSecret)
	formQuery.Add("code", code)
	formQuery.Add("grant_type", "authorization_code")
	return a.validateRequest(formQuery)
}

func (a *appleAuth) validateCodeWithRedirectURI(clientSecret, code, redirectURI string) (*TokenResponse, error) {
	formQuery := make(url.Values)
	formQuery.Add("client_id", a.AppID)
	formQuery.Add("client_secret", clientSecret)
	formQuery.Add("code", code)
	formQuery.Add("grant_type", "authorization_code")
	formQuery.Add("redirect_uri", redirectURI)
	return a.validateRequest(formQuery)
}

func (a *appleAuth) validateRefreshToken(clientSecret, refreshToken string) (*TokenResponse, error) {
	formQuery := make(url.Values)
	formQuery.Add("client_id", a.AppID)
	formQuery.Add("client_secret", clientSecret)
	formQuery.Add("refresh_token", refreshToken)
	formQuery.Add("grant_type", "refresh_token")
	return a.validateRequest(formQuery)
}

func (a *appleAuth) validateRequest(formQuery url.Values) (*TokenResponse, error) {
	res, err := a.httpClient.PostForm(validationEndpoint, formQuery)
	if err != nil {
		return nil, err
	}
	defer func() {
		_ = res.Body.Close()
	}()

	if res.StatusCode != http.StatusOK {
		var errorResponseBody appleErrorResponseBody
		if err := json.NewDecoder(res.Body).Decode(&errorResponseBody); err != nil {
			return nil, err
		}
		switch errorResponseBody.Error {
		case string(ErrorResponseInvalidScope):
			return nil, ErrorResponseInvalidScope
		case string(ErrorResponseUnsupportedGrant):
			return nil, ErrorResponseUnsupportedGrant
		case string(ErrorResponseUnauthorizedClient):
			return nil, ErrorResponseUnauthorizedClient
		case string(ErrorResponseInvalidClient):
			return nil, ErrorResponseInvalidClient
		case string(ErrorResponseInvalidRequest):
			return nil, ErrorResponseInvalidRequest
		default:
			return nil, fmt.Errorf("unrecognized response error: %s", errorResponseBody.Error)
		}
	}

	var tokenResponse TokenResponse
	if err := json.NewDecoder(res.Body).Decode(&tokenResponse); err != nil {
		return nil, err
	}
	return &tokenResponse, nil
}

type AuthorizeURLConfig struct {
	ClientID     string
	RedirectURI  string
	State        string
	Nonce        string
	Scope        []string
	ResponseType string
	ResponseMode string
}

const (
	ResponseTypeCode      = "code"
	ResponseModeFormPost  = "form_post"
)

func AuthorizeURL(config AuthorizeURLConfig) string {
	params := url.Values{}
	params.Set("client_id", config.ClientID)
	params.Set("redirect_uri", config.RedirectURI)
	params.Set("state", config.State)
	params.Set("nonce", config.Nonce)
	params.Set("scope", strings.Join(config.Scope, " "))
	params.Set("response_type", config.ResponseType)
	params.Set("response_mode", config.ResponseMode)

	return "https://appleid.apple.com/auth/authorize?" + params.Encode()
}
