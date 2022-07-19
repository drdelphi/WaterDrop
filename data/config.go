package data

type AppConfig struct {
	StakingSC           string `json:"stakingSC"`
	Proxy               string `json:"proxy"`
	Indexer             string `json:"indexer"`
	StartTime           int64  `json:"startTime"`
	EndTime             int64  `json:"endTime"`
	BonusToken          string `json:"bonusToken"`
	BonusAmount         uint64 `json:"bonusAmount"`
	PerStakedAmount     uint64 `json:"perStakedAmount"`
	BonusWallet         string `json:"bonusWallet"`
	BonusWalletPassword string `json:"bonusWalletPassword"`
}
