package main

import (
	"bufio"
	"context"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"math"
	"math/big"
	"os"
	"strings"

	"github.com/DrDelphi/WaterDrop/data"
	"github.com/DrDelphi/WaterDrop/network"
	"github.com/ElrondNetwork/elrond-sdk-erdgo/blockchain"
	"github.com/ElrondNetwork/elrond-sdk-erdgo/builders"
	"github.com/ElrondNetwork/elrond-sdk-erdgo/interactors"
)

func main() {
	appConfig, err := loadConfig("config.json")
	if err != nil {
		panic("unable to load config file: " + err.Error())
	}

	netMan, err := network.NewNetworkManager(appConfig.Proxy, appConfig.Indexer)
	if err != nil {
		panic("error creating network manager: " + err.Error())
	}

	w := interactors.NewWallet()
	privateKey := make([]byte, 32)
	if strings.HasSuffix(appConfig.BonusWallet, ".pem") {
		privateKey, err = w.LoadPrivateKeyFromPemFile(appConfig.BonusWallet)
	} else {
		privateKey, err = w.LoadPrivateKeyFromJsonFile(appConfig.BonusWallet, appConfig.BonusWalletPassword)
	}
	if err != nil {
		panic("error opening wallet file")
	}

	token, err := netMan.GetTokenProperties(appConfig.BonusToken)
	if err != nil {
		panic("can not get token properties: " + err.Error())
	}

	delegations := make(map[string]float64)
	averageDelegations := make(map[string]float64)

	daySeconds := int64(24 * 60 * 60)
	fromTime := netMan.NetCfg.StartTime
	toTime := appConfig.StartTime + daySeconds
	days := 0
	for fromTime <= appConfig.EndTime {
		fmt.Printf("analyzing txs between %v and %v\n\r", fromTime, toTime)
		txs := make([]*data.ElasticEntry, 0)
		from := fromTime
		for {
			txs2, err := netMan.GetIndexedTxs(appConfig.StakingSC, 10000, from, toTime)
			if err != nil {
				panic("error retrieving txs from indexer: " + err.Error())
			}
			txs = append(txs, txs2...)
			if len(txs2) < 10000 {
				break
				// panic("elastic search limit reached")
			} else {
				from = int64(txs2[len(txs2)-1].Source.Timestamp) + 1
			}
		}
		fmt.Printf("%v txs\n\r", len(txs))
		for _, tx := range txs {
			if tx.Source.Status != "success" {
				continue
			}
			txData := string(tx.Source.Data)
			if txData == "delegate" {
				delegations[tx.Source.Sender] += str2float(tx.Source.Value, netMan.NetCfg.Denomination)
			}

			if strings.HasPrefix(txData, "unDelegate@") {
				value := strings.TrimPrefix(txData, "unDelegate@")
				iValue, _ := big.NewInt(0).SetString(value, 16)
				delegations[tx.Source.Sender] -= str2float(iValue.String(), netMan.NetCfg.Denomination)
			}

			if txData == "reDelegateRewards" {
				scrs, err := netMan.GetTxScrs(tx.Hash, 10)
				if err != nil {
					panic("can not get tx scrs: " + err.Error())
				}
				value := "0"
				for _, scr := range scrs {
					if scr.Source.Receiver != "erd1qqqqqqqqqqqqqqqpqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqplllst77y4l" || scr.Source.Sender != appConfig.StakingSC {
						continue
					}
					value = scr.Source.Value
				}
				delegations[tx.Source.Sender] += str2float(value, netMan.NetCfg.Denomination)
			}
		}
		for delegator, value := range delegations {
			averageDelegations[delegator] += value
		}
		days++
		if fromTime == netMan.NetCfg.StartTime {
			fromTime = appConfig.StartTime
		}
		fromTime += daySeconds
		toTime = fromTime + daySeconds
	}

	senderAddress, _ := w.GetAddressFromPrivateKey(privateKey)
	ep := blockchain.NewElrondProxy(appConfig.Proxy, nil)
	builder, _ := builders.NewTxBuilder(blockchain.NewTxSigner())
	ti, _ := interactors.NewTransactionInteractor(ep, builder)
	txArgs, _ := ep.GetDefaultTransactionArguments(context.Background(), senderAddress, netMan.NetCfg)
	txArgs.Value = "0"
	txArgs.GasLimit = 500000

	total := float64(0)
	count := 0
	for delegator, value := range averageDelegations {
		if value < 0 {
			delete(averageDelegations, delegator)
			continue
		}
		averageDelegations[delegator] = value / float64(days)
		total += math.Floor(float64(appConfig.BonusAmount) * averageDelegations[delegator] / float64(appConfig.PerStakedAmount))
		count++
	}

	// export data to csv
	fmt.Println("exporting data...")
	f, err := os.Create("output.csv")
	if err != nil {
		panic("can not create file output.csv: " + err.Error())
	}
	fmt.Fprintln(f, "Address,Average Stake,Last Staked,Token Balance")
	for delegator, value := range averageDelegations {
		b, _ := netMan.GetTokenBalance(delegator, appConfig.BonusToken, int(token.Decimals))
		fmt.Fprintf(f, "%s,%.2f,%.2f,%.2f\n", delegator, value, delegations[delegator], b)
	}
	f.Close()

	fmt.Printf("output.csv exported. You are about to send %.2f %s to %v delegators. Continue ? (y/n)", total, appConfig.BonusToken, count)
	reader := bufio.NewReader(os.Stdin)
	answear, _ := reader.ReadString('\n')
	if strings.TrimSpace(answear) != "y" {
		os.Exit(1)
	}

	// check balances
	balance, err := netMan.GetBalance(senderAddress.AddressAsBech32String())
	if err != nil {
		panic("can not get wallet balance: " + err.Error())
	}
	fees := 0.00015 * float64(count)
	if balance < fees {
		fmt.Printf("Insufficient funds. You have %.6f and you need %.6f eGLD\n\r", balance, fees)
		os.Exit(1)
	}
	balance, err = netMan.GetTokenBalance(senderAddress.AddressAsBech32String(), appConfig.BonusToken, int(token.Decimals))
	if err != nil {
		panic("can not get wallet token balance")
	}
	if balance < total {
		fmt.Printf("Insufficient funds. You have %.2f and you need %.2f %s\n\r", balance, total, appConfig.BonusToken)
		os.Exit(1)
	}

	for delegator, value := range averageDelegations {
		txArgs.RcvAddr = delegator
		txArgs.Data = []byte("ESDTTransfer@" + hex.EncodeToString([]byte(appConfig.BonusToken)) + "@" +
			float2hex(math.Floor(value*float64(appConfig.BonusAmount)/float64(appConfig.PerStakedAmount)), int(token.Decimals)))
		tx, _ := ti.ApplySignatureAndGenerateTx(privateKey, txArgs)
		ti.AddTransaction(tx)
		txArgs.Nonce++
	}
	_, err = ti.SendTransactionsAsBunch(context.Background(), 1000)
	if err != nil {
		fmt.Println(err)
	}
}

func loadConfig(fileName string) (*data.AppConfig, error) {
	bytes, err := ioutil.ReadFile(fileName)
	if err != nil {
		return nil, err
	}

	cfg := &data.AppConfig{}
	err = json.Unmarshal(bytes, cfg)
	if err != nil {
		return nil, err
	}

	return cfg, nil
}

func str2float(value string, decimals int) float64 {
	f, ok := big.NewFloat(0).SetString(value)
	if !ok {
		return 0
	}

	d := big.NewFloat(10)
	for i := 0; i < decimals; i++ {
		f.Quo(f, d)
	}
	res, _ := f.Float64()

	return res
}

func float2hex(value float64, decimals int) string {
	fValue := big.NewFloat(value)
	d := big.NewFloat(10)
	for i := 0; i < decimals; i++ {
		fValue.Mul(fValue, d)
	}
	iValue, _ := fValue.Int(nil)

	return hex.EncodeToString(iValue.Bytes())
}
