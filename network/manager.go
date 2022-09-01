package network

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"strconv"
	"strings"

	"github.com/DrDelphi/WaterDrop/data"
	"github.com/DrDelphi/WaterDrop/utils"
	"github.com/ElrondNetwork/elrond-sdk-erdgo/blockchain"
	sdkData "github.com/ElrondNetwork/elrond-sdk-erdgo/data"
)

type NetworkManager struct {
	Proxy                 blockchain.Proxy
	proxyAddress, indexer string
	NetCfg                *sdkData.NetworkConfig
}

func NewNetworkManager(proxy string, indexer string) (*NetworkManager, error) {
	var err error
	nm := &NetworkManager{}

	nm.Proxy = blockchain.NewElrondProxy(proxy, nil)
	nm.proxyAddress = proxy
	nm.indexer = indexer
	nm.NetCfg, err = nm.Proxy.GetNetworkConfig(context.Background())
	if err != nil {
		return nil, err
	}

	return nm, nil
}

func (nm *NetworkManager) GetIndexedTxs(toAddress string, size int, fromTime int64, toTime int64) ([]*data.ElasticEntry, error) {
	endpoint := fmt.Sprintf("%s/transactions/_search?size=%v&q=receiver:%s", nm.indexer, size, toAddress)
	endpoint += fmt.Sprintf("%%20AND%%20timestamp:>%v%%20AND%%20timestamp:<%v&sort=timestamp:asc", fromTime, toTime)
	bytes, err := utils.GetHTTP(endpoint)
	if err != nil {
		return nil, err
	}

	list := data.ElasticResult{}
	err = json.Unmarshal(bytes, &list)
	if err != nil {
		return nil, err
	}

	return list.Hits.Hits, nil
}

func (nm *NetworkManager) GetIndexedTxs2(toAddress, fromAddress string, size int, fromTime int64, toTime int64) ([]*data.ElasticEntry, error) {
	endpoint := fmt.Sprintf("%s/transactions/_search?size=%v&q=receiver:%s%%20AND%%20sender:%s", nm.indexer, size, toAddress, fromAddress)
	endpoint += fmt.Sprintf("%%20AND%%20timestamp:>%v%%20AND%%20timestamp:<%v&sort=timestamp:asc", fromTime, toTime)
	bytes, err := utils.GetHTTP(endpoint)
	if err != nil {
		return nil, err
	}

	list := data.ElasticResult{}
	err = json.Unmarshal(bytes, &list)
	if err != nil {
		return nil, err
	}

	return list.Hits.Hits, nil
}

func (nm *NetworkManager) GetTxScrs(txHash string, size int) ([]*data.ElasticEntry, error) {
	endpoint := fmt.Sprintf("%s/scresults/_search?size=%v&q=originalTxHash:%s", nm.indexer, size, txHash)
	bytes, err := utils.GetHTTP(endpoint)
	if err != nil {
		return nil, err
	}

	list := data.ElasticResult{}
	err = json.Unmarshal(bytes, &list)
	if err != nil {
		return nil, err
	}

	return list.Hits.Hits, nil
}

func (nm *NetworkManager) GetTokenProperties(ticker string) (*data.ESDT, error) {
	req := &sdkData.VmValueRequest{
		Address:  "erd1qqqqqqqqqqqqqqqpqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqzllls8a5w6u",
		FuncName: "getTokenProperties",
		Args:     []string{hex.EncodeToString([]byte(ticker))},
	}
	res, err := nm.Proxy.ExecuteVMQuery(context.Background(), req)
	if err != nil {
		return nil, err
	}

	if len(res.Data.ReturnData) < 6 {
		return nil, errors.New("invalid get token properties response")
	}

	token := &data.ESDT{}
	token.Ticker = ticker
	token.ShortTicker = strings.Split(ticker, "-")[0]
	token.Name = string(res.Data.ReturnData[0])
	decimalsStr := strings.TrimPrefix(string(res.Data.ReturnData[5]), "NumDecimals-")
	token.Decimals, err = strconv.ParseUint(decimalsStr, 10, 64)

	if err != nil {
		return nil, errors.New("invalid token decimals")
	}

	return token, nil
}

func (nm *NetworkManager) GetBalance(address string) (float64, error) {
	addr, err := sdkData.NewAddressFromBech32String(address)
	if err != nil {
		return 0, err
	}

	account, err := nm.Proxy.GetAccount(context.Background(), addr)
	if err != nil {
		return 0, err
	}

	return account.GetBalance(nm.NetCfg.Denomination)
}

func (nm *NetworkManager) GetTokenBalance(address, tokenIdentifier string) (float64, error) {
	endpoint := fmt.Sprintf("%s/address/%s/esdt/%s", nm.proxyAddress, address, tokenIdentifier)
	bytes, err := utils.GetHTTP(endpoint)
	if err != nil {
		return 0, err
	}

	res := data.EsdtBalanceResponse{}
	err = json.Unmarshal(bytes, &res)
	if err != nil {
		return 0, err
	}

	fBalance, ok := big.NewFloat(0).SetString(res.Data.TokenData.Balance)
	if !ok {
		return 0, errors.New("invalid response")
	}

	prop, err := nm.GetTokenProperties(tokenIdentifier)
	if err != nil {
		return 0, err
	}

	fDenom := big.NewFloat(1)
	for i := 0; i < int(prop.Decimals); i++ {
		fDenom.Mul(fDenom, big.NewFloat(10))
	}

	fBalance.Quo(fBalance, fDenom)
	ret, _ := fBalance.Float64()

	return ret, nil
}
