package socialauth

// stubAppleClient stands in for the Apple token endpoint. Only construction and
// URL building are exercised here; the exchange needs Apple's live keys.
type stubAppleClient struct{}

func (stubAppleClient) ValidateCode(string) (*TokenResponse, error)    { return nil, nil }
func (stubAppleClient) ValidateCodeWithRedirectURI(string, string) (*TokenResponse, error) {
	return nil, nil
}
func (stubAppleClient) ValidateRefreshToken(string) (*TokenResponse, error) { return nil, nil }
