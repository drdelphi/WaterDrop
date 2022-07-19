package data

type ESDT struct {
	Name        string
	Ticker      string
	ShortTicker string
	Decimals    uint64
}

type TokensList struct {
	Data struct {
		Tokens []string `json:"tokens"`
	} `json:"data"`
}

type EsdtBalanceResponse struct {
	Data struct {
		TokenData struct {
			Balance string `json:"balance"`
		} `json:"tokenData"`
	} `json:"data"`
	Error string `json:"error"`
}
