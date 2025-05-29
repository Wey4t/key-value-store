package types

type Key struct {
	Writers   []string `json:"writers"`
	Readers   []string `json:"readers"`
	Copyfroms []string `json:"copyfroms"`
	Copytos   []string `json:"copytos"`
	Indirects []string `json:"indirects"`
	Owner     string   `json:"owner"`
}
