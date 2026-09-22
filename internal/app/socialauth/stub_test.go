package socialauth

import apple "github.com/meszmate/apple-go"

// stubAppleClient stands in for the Apple token endpoint. Only construction and
// URL building are exercised here; the exchange needs Apple's live keys.
type stubAppleClient struct{}

func (stubAppleClient) ValidateCode(string) (*apple.TokenResponse, error) { return nil, nil }
func (stubAppleClient) ValidateCodeWithRedirectURI(string, string) (*apple.TokenResponse, error) {
	return nil, nil
}
func (stubAppleClient) ValidateRefreshToken(string) (*apple.TokenResponse, error) { return nil, nil }
