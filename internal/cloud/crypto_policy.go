package cloud

import (
	"context"
	"math/big"
	"net/http"
	"net/url"
	"strings"
)

// validateLiveCryptoPolicy rejects stale tenant fees or an inactive/mismatched
// asset before a new address is offered to a customer. No public RPC is trusted.
func (s *Server) validateLiveCryptoPolicy(ctx context.Context) error {
	var response struct {
		Data []struct {
			Active   bool   `json:"isActive"`
			Fixed    string `json:"depositProviderFeeFixed"`
			Percent  string `json:"depositProviderFeePer"`
			Treasury string `json:"tenantConsolidationAddress"`
			Token    *struct {
				Name     string `json:"name"`
				Decimals int    `json:"decimal"`
				Address  string `json:"tokenContractAddress"`
				Network  string `json:"blockchainName"`
				Active   bool   `json:"isActive"`
				Type     string `json:"type"`
			} `json:"tokenConfig"`
		} `json:"data"`
		Meta struct {
			HasNext bool `json:"hasNextPage"`
		} `json:"meta"`
	}
	query := url.Values{"tokenAddress": {s.Config.CryptoRailsTokenAddress}, "blockchainName": {s.Config.CryptoRailsNetwork}, "take": {"100"}, "page": {"1"}, "isActive": {"true"}}
	if _, _, err := s.cryptoCall(ctx, http.MethodGet, "/api/tenant-token-statuses?"+query.Encode(), nil, &response); err != nil {
		return err
	}
	if response.Meta.HasNext || len(response.Data) != 1 {
		return ErrUnavailable
	}
	p := response.Data[0]
	equal := func(a, b string) bool {
		if s.Config.CryptoRailsVMType == "EVM" {
			return strings.EqualFold(a, b)
		}
		return a == b
	}
	if !p.Active || !equal(p.Treasury, s.Config.CryptoRailsTreasuryAddress) {
		return ErrUnavailable
	}
	// The provider schema allows tokenConfig to be omitted. Its exact token and
	// network filters scope the returned tenant policy; operator-reviewed token
	// decimals are required in configuration rather than guessed from the symbol.
	if p.Token != nil && (!p.Token.Active || p.Token.Type != "ERC20" || p.Token.Decimals != s.Config.CryptoRailsTokenDecimals || p.Token.Name != s.Config.CryptoRailsToken || p.Token.Network != s.Config.CryptoRailsNetwork || !equal(p.Token.Address, s.Config.CryptoRailsTokenAddress)) {
		return ErrUnavailable
	}

	fixed, err := cryptoUnits(p.Fixed)
	if err != nil {
		return ErrUnavailable
	}
	configured, err := cryptoUnits(s.Config.CryptoRailsDepositFixed)
	if err != nil || fixed.Cmp(configured) != 0 {
		return ErrUnavailable
	}
	percent, err := cryptoUnits(p.Percent)
	if err != nil {
		return ErrUnavailable
	}
	// One basis point is 0.01 percent, represented exactly in 18 decimal units.
	expected := new(big.Int).Mul(big.NewInt(s.Config.CryptoRailsDepositFeeBPS), new(big.Int).Div(new(big.Int).Set(cryptoScale), big.NewInt(100)))
	if percent.Cmp(expected) != 0 {
		return ErrUnavailable
	}
	return nil
}
