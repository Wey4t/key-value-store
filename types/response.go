package types

type Code int

const (
	OK Code = iota
	FAIL
)

type Response struct {
	Status    Code        `json:"status"`
	Val       interface{} `json:"val"`
	Uid       string      `json:"uid"`
	Writers   []string    `json:"writers"`
	Readers   []string    `json:"readers"`
	Copytos   []string    `json:"copytos"`
	Copyfroms []string    `json:"copyfroms"`
	Indirects []string    `json:"indirects"`
	R         []string    `json:"r(k)"`
	W         []string    `json:"w(k)"`
	C_src     []string    `json:"c_src(k)"`
	C_dst     []string    `json:"c_dst(k)"`
}
